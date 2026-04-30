"""Direct HTTP LLM client using httpx — bypasses OpenAI SDK issues on Windows."""

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
    """Call an OpenAI-compatible LLM API directly via httpx.

    Returns a langchain AIMessage with content and/or tool_calls.
    """
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
        logger.info("POST %s (model=%s, %d messages, %d tools)", url, model, len(messages), len(tools or []))
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

    content = msg.get("content") or ""

    return AIMessage(
        content=content,
        tool_calls=tool_calls,
    )
