---
name: a2ui-mcp-integration
description: >
  Integrate A2UI (Agent-to-User Interface) into any agent project using MCP tools and the A2UI React SDK.
  Use this skill when building AI agent frontends that generate interactive UIs, when connecting an LLM
  agent to A2UI MCP tools, or when rendering A2UI JSON in a React frontend. Covers the full stack:
  MCP server setup, agent tool integration, frontend rendering, and common pitfalls.
compatibility: Requires A2UI MCP server (Go binary), any MCP-compatible agent framework, React 18+
---

# A2UI MCP Integration Guide

## Architecture Overview

```
User Chat  -->  Agent (any framework)  -->  A2UI MCP Server (port 8080)
                                              |
                                        A2UI JSON messages
                                              |
                  React Frontend  <-----------+
                  (@a2ui SDK)
```

You only need two things:
1. **A2UI MCP Server** -- exposes tools for building UI components
2. **A2UI React SDK** -- renders the A2UI JSON output

Any agent framework (LangChain, CrewAI, Claude, OpenAI Assistants, custom) can be the middle layer as long as it supports MCP tool calling.

## Step 1: Start the A2UI MCP Server

```bash
cd tools/a2ui-mcp-server
go build -o a2ui-mcp-server.exe .
./a2ui-mcp-server.exe -addr :8080 -spec ../specification/v0_9/json
```

The server exposes MCP over StreamableHTTP at `http://localhost:8080/mcp`.

It registers three categories of tools:
- **Protocol**: `create_surface`, `update_data_model`, `delete_surface`
- **Component**: `create_Text`, `create_Button`, `create_TextField`, `create_Row`, etc. (one per catalog component)
- **Render**: `render_ui` -- assembles all components into final A2UI JSON

## Step 2: Connect Your Agent to MCP

Any MCP-compatible agent framework can call A2UI tools. The workflow the LLM should follow:

1. Call `create_surface` with a surface_id (e.g. "main")
2. Call `create_*` tools to build UI components, each with a unique `id`
3. One component must be `id="root"` as the tree root
4. Call `render_ui` to produce the final A2UI JSON

### System Prompt Template

```
You are an A2UI assistant. You build user interfaces by calling A2UI MCP tools.

Workflow:
1. Call `create_surface` with a surface_id (e.g. "main").
2. Create UI components using `create_*` tools. Each needs a unique `id`.
   Layout components (Row, Column, Card, List) hold children via `children` or `child`.
3. Call `render_ui` to assemble and return the final A2UI JSON.

Rules:
- Always create a surface first.
- One component must have id="root" as the tree root.
- Use Row/Column for layout, nest components inside.
- After creating all components, call render_ui to produce the output.
```

### MCP Connection URL

The MCP endpoint is: `http://localhost:8080/mcp`

**Critical**: You must include the `/mcp` path. Connecting to `http://localhost:8080` will fail.

## Step 3: Render A2UI in React

Install peer dependencies (React 18+). The A2UI SDK packages are resolved from the monorepo via Vite aliases or published npm packages.

### Key Imports

```tsx
import { A2uiSurface, basicCatalog, MarkdownContext } from '@a2ui/react/v0_9';
import {
  A2uiClientMessage, MessageProcessor, SurfaceModel,
  ReactComponentImplementation
} from '@a2ui/web_core/v0_9';
import { renderMarkdown } from '@a2ui/markdown-it';
```

### Core Integration Pattern

```tsx
// 1. Create processor ONCE (useMemo)
const processor = useMemo(
  () => new MessageProcessor<ReactComponentImplementation>(
    [basicCatalog],
    (action) => actionHandlerRef.current(action)  // action callback
  ),
  []
);

// 2. Subscribe to surface events
useEffect(() => {
  const sub1 = processor.onSurfaceCreated((s) => setSurfaces(prev => [...prev, s]));
  const sub2 = processor.onSurfaceDeleted((id) => setSurfaces(prev => prev.filter(s => s.id !== id)));
  return () => { sub1.unsubscribe(); sub2.unsubscribe(); };
}, [processor]);

// 3. Process A2UI messages from agent response
const a2uiMsgs = extractA2UIMessages(agentResponse); // your extraction logic
processor.processMessages(a2uiMsgs);

// 4. Render surfaces
<MarkdownContext.Provider value={renderMarkdown}>
  {surfaces.map(s => <A2uiSurface key={s.id} surface={s} />)}
</MarkdownContext.Provider>
```

See `references/react-integration.md` for the full working example.

## Step 4: Handle User Actions (Interactivity)

When a user interacts with a rendered component (e.g., clicks a button with an `action` binding), the processor fires the action callback. Send this back to your agent as a user message:

```tsx
actionHandlerRef.current = async (action) => {
  const actionMsg = `[User Action] event: ${action.name}, component: ${action.sourceComponentId}, data: ${JSON.stringify(action.context)}`;
  // Send actionMsg back to agent and process the new A2UI response
};
```

The agent receives this message, decides what to do (update data model, create new components, etc.), and calls `render_ui` again to produce an updated UI.

## Critical Pitfalls

### 1. Only `render_ui` Produces A2UI Messages

The `create_surface`, `create_Text`, `create_Button` etc. tools return confirmation fragments -- NOT renderable A2UI JSON. Only `render_ui` produces the final A2UI messages (createSurface + updateComponents + updateDataModel) that the frontend can render.

If you extract messages from every tool result, you will get "Surface already exists" errors because `create_surface` returns a surface definition fragment.

### 2. Controlled Inputs Need `{path: "..."}` Bindings

TextField, CheckBox, ChoicePicker use **controlled inputs**. Their value properties must be a binding object `{path: "/data/xxx/value"}`, not a plain string. The MCP server auto-generates these path bindings for `DynamicString`/`DynamicBoolean`/`DynamicNumber` properties, but if the LLM passes a plain string value, `setValue()` actions will not work -- the component appears but user input has no effect.

### 3. `useRef` Pattern for Action Handlers

Action callbacks that reference React state must use `useRef` to avoid stale closures:

```tsx
const actionHandlerRef = useRef<(action: ActionPayload) => void>(() => {});
// Update ref in useEffect (not in render)
useEffect(() => {
  actionHandlerRef.current = async (action) => { /* use current state here */ };
}, [deps]);
```

If you pass the handler directly to `MessageProcessor` constructor, it captures the initial empty state and never updates.

### 4. OpenAI SDK Hangs on Windows

The OpenAI Python SDK (v2.x) has a known issue on Windows where `SyncHttpxClientWrapper.send` never completes. If your agent uses `langchain_openai.ChatOpenAI` or the OpenAI SDK directly, it may hang indefinitely.

**Fix**: Use direct `httpx` calls instead. See `references/llm-httpx-bypass.md` for the implementation.

### 5. `langchain-deepseek` Requires Env Var

`ChatDeepSeek` requires `DEEPSEEK_API_KEY` as an environment variable, even when you pass `api_key` as a parameter. Set it before creating the LLM instance:

```python
os.environ["DEEPSEEK_API_KEY"] = config.llm_api_key
```

### 6. camelCase / snake_case Normalization

The gRPC-Gateway may serialize response fields in camelCase (`sessionId`, `a2uiMessages`) while the protobuf definition uses snake_case (`session_id`, `a2ui_messages`). Always normalize both in your client:

```ts
session_id: raw.session_id ?? raw.sessionId ?? '',
a2ui_messages: raw.a2ui_messages ?? raw.a2uiMessages ?? [],
```

### 7. A2UI Message Deduplication

If the agent emits multiple messages for the same surface (e.g., two `createSurface` for "main"), deduplicate by `kind:surfaceId` key (last-wins) before calling `processor.processMessages()`.

### 8. `args_schema` May Be Dict or Pydantic Model

When converting MCP tools for LLM function calling, `tool.args_schema` may be either a Pydantic model (with `.model_json_schema()`) or a plain dict. Handle both:

```python
schema = raw.model_json_schema() if hasattr(raw, "model_json_schema") else raw
```

### 9. MCP Server Catalog Path

The MCP server auto-discovers catalog files from the binary location. If the binary can't find catalogs, pass `-spec` explicitly pointing to the `specification/v0_9/json` directory.

### 10. MarkdownContext Provider

If you want markdown rendering inside A2UI components (Text with `variant: "markdown"`), wrap your surfaces in `<MarkdownContext.Provider value={renderMarkdown}>`.

### 11. `render_ui` Returns `EmbeddedResource`, Not Plain Text

The `render_ui` tool returns the A2UI JSON array inside an `EmbeddedResource` (MIME type `application/json+a2ui`), not in a standard `TextContent`. The MCP tool result contains two content blocks:

```
Content[0]: TextContent    → "Rendered N components..." (summary, NOT the data)
Content[1]: EmbeddedResource → { uri: "a2ui://...", mimeType: "application/json+a2ui", text: "[...actual JSON...]" }
```

**The problem**: Default MCP text extractors (e.g., LangChain's `block.text`) may summarize `EmbeddedResource` as `[MCP returned embedded resource (application/json+a2ui).]` instead of returning the raw JSON. Your frontend `JSON.parse(m.json)` will then fail with a syntax error.

**Fix**: When extracting A2UI messages from tool results, you **must** specifically check for `EmbeddedResource` content blocks and read their `.text` field. Do not rely on generic text extraction:

```python
# ✅ Correct: handle EmbeddedResource explicitly
for block in tool_result.content:
    if hasattr(block, 'text') and isinstance(block.text, str):
        text = block.text
        if text.startswith('[') or text.startswith('{'):
            parsed = json.loads(text)
            # ... extract A2UI messages
```

The `TextContent` summary starts with `"Rendered"` — it will never parse as valid A2UI JSON. Only the `EmbeddedResource.text` contains the actual data.

## Verification Checklist

- [ ] MCP server running at `http://localhost:8080/mcp` (not `/`)
- [ ] Agent connects to MCP and loads all tools (should be ~18-22 tools)
- [ ] Agent follows create_surface -> create_* -> render_ui workflow
- [ ] Agent extracts A2UI JSON from `EmbeddedResource.text`, not from `TextContent` summary
- [ ] Frontend wraps surfaces in `<MarkdownContext.Provider>`
- [ ] `MessageProcessor` created once with `useMemo`
- [ ] Action handler uses `useRef` pattern
- [ ] A2UI messages deduplicated before `processMessages()`
- [ ] Response fields normalized for camelCase/snake_case
