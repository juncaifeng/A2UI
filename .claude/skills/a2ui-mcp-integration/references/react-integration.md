# React Frontend Integration -- Full Working Example

This is the complete React component that integrates with an A2UI agent backend.

## Dependencies

```json
{
  "dependencies": {
    "react": "^18.3.0",
    "react-dom": "^18.3.0"
  }
}
```

The `@a2ui/*` packages are resolved via Vite aliases (monorepo) or npm packages:

```ts
// vite.config.ts -- monorepo alias example
const repoRoot = path.resolve(__dirname, '../../..');  // adjust to your layout
export default defineConfig({
  resolve: {
    alias: {
      '@a2ui/react/v0_9':     path.join(repoRoot, 'renderers/react/src/v0_9'),
      '@a2ui/web_core/v0_9':  path.join(repoRoot, 'renderers/web_core/src/v0_9'),
      '@a2ui/markdown-it':    path.join(repoRoot, 'renderers/markdown/markdown-it/src/markdown'),
    },
  },
  server: {
    proxy: { '/v1': 'http://localhost:8081' },  // proxy to your agent backend
  },
});
```

## Full App.tsx

```tsx
import { useState, useEffect, useCallback, useMemo, useRef, FormEvent } from 'react';
import {
  A2uiSurface, basicCatalog, MarkdownContext,
} from '@a2ui/react/v0_9';
import {
  A2uiClientMessage, MessageProcessor, SurfaceModel,
  ReactComponentImplementation,
} from '@a2ui/web_core/v0_9';
import { renderMarkdown } from '@a2ui/markdown-it';

type ActionPayload = {
  name: string;
  surfaceId: string;
  sourceComponentId: string;
  context: Record<string, unknown>;
};

type A2UIMessage = Record<string, unknown> & { version?: string };

interface ChatResponse {
  session_id: string;
  text: string;
  a2ui_messages: { kind: string; json: string }[];
}

// --- Extract A2UI messages from agent response ---
function extractA2UIMessages(response: ChatResponse): A2UIMessage[] {
  if (!response.a2ui_messages || !Array.isArray(response.a2ui_messages)) return [];
  return response.a2ui_messages.map((m) => JSON.parse(m.json));
}

// --- Process and deduplicate A2UI messages ---
function processA2UI(
  response: ChatResponse,
  processor: MessageProcessor<ReactComponentImplementation>,
): { kinds: string[] } {
  const a2uiMsgs = extractA2UIMessages(response);
  if (a2uiMsgs.length === 0) return { kinds: [] };

  // Deduplicate by kind:surfaceId (last-wins)
  const seen = new Map<string, A2UIMessage>();
  for (const msg of a2uiMsgs) {
    const kind = Object.keys(msg).find((k) => k !== 'version') || '';
    const payload = (msg as Record<string, unknown>)[kind] as Record<string, unknown> | undefined;
    const surfaceId = (payload?.surfaceId as string) || 'default';
    seen.set(`${kind}:${surfaceId}`, msg);
  }
  const deduped = Array.from(seen.values());
  try { processor.processMessages(deduped); } catch (err) { console.warn('processMessages error:', err); }
  return { kinds: deduped.map((m) => Object.keys(m).find((k) => k !== 'version') || 'unknown') };
}

export function App() {
  const [sessionId, setSessionId] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [input, setInput] = useState('');
  const [agentText, setAgentText] = useState('');
  const [surfaces, setSurfaces] = useState<SurfaceModel<ReactComponentImplementation>[]>([]);

  const sessionIdRef = useRef(sessionId);
  sessionIdRef.current = sessionId;
  const actionHandlerRef = useRef<(action: ActionPayload) => void>(() => {});

  // 1. Create processor ONCE -- passes action callback via ref
  const processor = useMemo(() => {
    return new MessageProcessor<ReactComponentImplementation>(
      [basicCatalog],
      (action: ActionPayload) => actionHandlerRef.current(action),
    );
  }, []);

  // 2. Subscribe to surface create/delete events
  useEffect(() => {
    const sub1 = processor.onSurfaceCreated(
      (surface: SurfaceModel<ReactComponentImplementation>) => setSurfaces(prev => [...prev, surface])
    );
    const sub2 = processor.onSurfaceDeleted(
      (id: string) => setSurfaces(prev => prev.filter(s => s.id !== id))
    );
    return () => { sub1.unsubscribe(); sub2.unsubscribe(); };
  }, [processor]);

  // 3. Action handler -- sends user interaction back to agent
  useEffect(() => {
    actionHandlerRef.current = async (action: ActionPayload) => {
      const actionMsg = `[User Action] event: ${action.name}, component: ${action.sourceComponentId}, data: ${JSON.stringify(action.context)}`;
      setLoading(true);
      setError(null);
      try {
        const res = await fetch('/v1/chat', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ message: actionMsg, session_id: sessionIdRef.current }),
        });
        const raw = await res.json();
        const response: ChatResponse = {
          session_id: raw.session_id ?? raw.sessionId ?? '',
          text: raw.text ?? '',
          a2ui_messages: raw.a2ui_messages ?? raw.a2uiMessages ?? [],
        };
        setSessionId(response.session_id);
        setAgentText(response.text);
        processA2UI(response, processor);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setLoading(false);
      }
    };
  }, [processor]);

  // 4. Form submit handler
  const handleSubmit = useCallback(async (e: FormEvent) => {
    e.preventDefault();
    if (!input.trim() || loading) return;
    setLoading(true);
    setError(null);
    setAgentText('');
    // Clear old surfaces
    Array.from(processor.model.surfacesMap.keys()).forEach(id => processor.model.deleteSurface(id));
    setSurfaces([]);

    try {
      const res = await fetch('/v1/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: input, session_id: sessionIdRef.current || undefined }),
      });
      const raw = await res.json();
      const response: ChatResponse = {
        session_id: raw.session_id ?? raw.sessionId ?? '',
        text: raw.text ?? '',
        a2ui_messages: raw.a2ui_messages ?? raw.a2uiMessages ?? [],
      };
      setSessionId(response.session_id);
      setAgentText(response.text);
      processA2UI(response, processor);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [input, loading, processor]);

  return (
    <MarkdownContext.Provider value={renderMarkdown}>
      <form onSubmit={handleSubmit}>
        <textarea value={input} onChange={e => setInput(e.target.value)} disabled={loading} />
        <button type="submit" disabled={loading}>{loading ? '...' : 'Send'}</button>
      </form>
      {error && <div className="error">{error}</div>}
      {agentText && <p>{agentText}</p>}
      {surfaces.map(s => <A2uiSurface key={s.id} surface={s} />)}
    </MarkdownContext.Provider>
  );
}
```

## Key Points

- `MessageProcessor` is created with `useMemo` (runs once)
- Action callback goes through `useRef` to avoid stale closures
- `onSurfaceCreated` / `onSurfaceDeleted` are observable subscriptions -- always unsubscribe in cleanup
- `processMessages()` takes an array of A2UI message objects (the `createSurface`, `updateComponents`, `updateDataModel` messages)
- `<MarkdownContext.Provider>` enables markdown rendering in Text components
