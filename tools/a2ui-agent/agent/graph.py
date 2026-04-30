"""LangGraph agent — ReAct loop with A2UI MCP tools."""

from __future__ import annotations

import json
import logging
from typing import Any

from langchain_core.messages import SystemMessage
from langchain_deepseek import ChatDeepSeek
from langgraph.graph import START, StateGraph
from langgraph.graph.message import MessagesState
from langgraph.prebuilt import ToolNode, tools_condition

from agent.config import Config

logger = logging.getLogger("a2ui-agent")

# System prompt instructing the agent how to use A2UI tools
SYSTEM_PROMPT = """\
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
- Keep the UI simple and focused.
"""


async def create_graph(tools: list, config: Config):
    """Build and compile the LangGraph agent graph.

    Graph topology:
        START → call_model → tools_condition
                              ├── tools → call_model (loop)
                              └── END
    """

    async def call_model(state: MessagesState) -> dict[str, Any]:
        llm = ChatDeepSeek(
            model=config.llm_model,
            api_key=config.llm_api_key,
            base_url=config.llm_base_url,
        )
        llm_with_tools = llm.bind_tools(tools)

        messages = state["messages"]
        if not any(isinstance(m, SystemMessage) for m in messages):
            messages = [SystemMessage(content=SYSTEM_PROMPT)] + list(messages)

        logger.info("LLM thinking... (%d messages in context)", len(messages))
        response = await llm_with_tools.ainvoke(messages)

        # Log what the LLM decided
        if response.content:
            logger.info("LLM replied: %s", response.content[:300])
        if response.tool_calls:
            for tc in response.tool_calls:
                logger.info(
                    "LLM wants to call: %s(%s)",
                    tc.get("name", "?"),
                    json.dumps(tc.get("args", {}), ensure_ascii=False, default=str)[:300],
                )
        if not response.content and not response.tool_calls:
            logger.warning("LLM returned empty response (no text, no tool calls)")

        return {"messages": [response]}

    tool_node = ToolNode(tools)

    builder = StateGraph(MessagesState)
    builder.add_node("call_model", call_model)
    builder.add_node("tools", tool_node)
    builder.add_edge(START, "call_model")
    builder.add_conditional_edges("call_model", tools_condition)
    builder.add_edge("tools", "call_model")

    return builder.compile()
