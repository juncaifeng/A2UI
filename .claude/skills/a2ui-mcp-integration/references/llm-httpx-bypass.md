# Bypassing OpenAI SDK Hang on Windows

## Problem

The OpenAI Python SDK v2.x has a known issue on Windows where the internal httpx wrapper
(`SyncHttpxClientWrapper.send`) never completes. Even when raw `httpx.post()` works fine
in 1-2 seconds, the SDK hangs indefinitely.

This affects both `openai.OpenAI` and `langchain_openai.ChatOpenAI` (which uses the SDK internally).

## Solution: Direct httpx Client

Replace the SDK with a direct httpx-based LLM client that calls the OpenAI-compatible API:

```python
"""Direct HTTP LLM client using httpx -- bypasses OpenAI SDK issues on Windows."""

from __future__ import annotations
import json
import logging
from typing import Any
import httpx
from langchain_core.messages import AIMessage

logger = logging.getLogger("a2ui-agent")


async def call_llm(
    *,
    base_url: str,
    api_key: str,
    model: str,
    messages: list[dict[str, Any]],
    tools: list[dict[str, Any]] | None = None,
    timeout: float = 120,
) -> AIMessage:
    """Call an OpenAI-compatible LLM API directly via httpx."""
    payload: dict[str, Any] = {
        "model": model,
        "messages": messages,
    }
    if tools:
        payload["tools"] = tools

    url = f"{base_url.rstrip('/')}/chat/completions"
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }

    async with httpx.AsyncClient(timeout=timeout) as client:
        resp = await client.post(url, headers=headers, json=payload)
        resp.raise_for_status()

    data = resp.json()
    choice = data["choices"][0]
    msg = choice["message"]

    # Parse tool calls
    tool_calls = []
    for tc in msg.get("tool_calls") or []:
        fn = tc["function"]
        args = fn.get("arguments", "{}")
        if isinstance(args, str):
            args = json.loads(args)
        tool_calls.append({
            "id": tc.get("id", ""),
            "name": fn["name"],
            "args": args,
        })

    return AIMessage(content=msg.get("content") or "", tool_calls=tool_calls)
```

## Converting LangChain Messages to OpenAI Format

When using `call_llm` with LangGraph, convert messages to dicts:

```python
from langchain_core.messages import AIMessage, HumanMessage, SystemMessage, ToolMessage

def messages_to_dicts(messages: list) -> list[dict[str, Any]]:
    out = []
    for m in messages:
        if isinstance(m, SystemMessage):
            out.append({"role": "system", "content": m.content})
        elif isinstance(m, HumanMessage):
            out.append({"role": "user", "content": m.content})
        elif isinstance(m, AIMessage):
            d = {"role": "assistant", "content": m.content or ""}
            if m.tool_calls:
                d["tool_calls"] = [
                    {
                        "id": tc["id"],
                        "type": "function",
                        "function": {
                            "name": tc["name"],
                            "arguments": json.dumps(tc["args"], ensure_ascii=False, default=str),
                        },
                    }
                    for tc in m.tool_calls
                ]
            out.append(d)
        elif isinstance(m, ToolMessage):
            out.append({"role": "tool", "tool_call_id": m.tool_call_id, "content": m.content or ""})
        else:
            out.append({"role": "user", "content": str(m.content)})
    return out
```

## Converting LangChain Tools to OpenAI Format

```python
def tools_to_dicts(tools: list) -> list[dict[str, Any]]:
    out = []
    for tool in tools:
        schema = {}
        if hasattr(tool, "args_schema") and tool.args_schema:
            raw = tool.args_schema
            schema = raw.model_json_schema() if hasattr(raw, "model_json_schema") else raw
        out.append({
            "type": "function",
            "function": {
                "name": tool.name,
                "description": tool.description or "",
                "parameters": schema,
            },
        })
    return out
```

## Usage in LangGraph

```python
from agent.llm_httpx import call_llm

async def call_model(state: MessagesState) -> dict[str, Any]:
    messages = state["messages"]
    if not any(isinstance(m, SystemMessage) for m in messages):
        messages = [SystemMessage(content=SYSTEM_PROMPT)] + list(messages)

    response = await call_llm(
        base_url=config.llm_base_url,
        api_key=config.llm_api_key,
        model=config.llm_model,
        messages=messages_to_dicts(messages),
        tools=tools_to_dicts(tools),
        timeout=120,
    )
    return {"messages": [response]}
```
