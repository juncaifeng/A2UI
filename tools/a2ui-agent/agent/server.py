"""A2UI Agent gRPC server — exposes LangGraph agent via gRPC/protobuf."""

from __future__ import annotations

import asyncio
import json
import logging
import sys
import uuid
from concurrent import futures
from pathlib import Path
from typing import AsyncIterator

import grpc

# Add generated stubs to import path
_gen_py = str(Path(__file__).resolve().parent.parent / "gen" / "py")
if _gen_py not in sys.path:
    sys.path.insert(0, _gen_py)

from v1 import agent_pb2, agent_pb2_grpc

from agent.config import Config
from agent.graph import create_graph
from agent.mcp_client import create_mcp_client

from langchain_core.messages import AIMessage, BaseMessage, HumanMessage, ToolMessage
from langchain_mcp_adapters.tools import load_mcp_tools

logger = logging.getLogger("a2ui-agent")

# Map A2UI message type keys to proto kind values
_A2UI_KIND_MAP = {
    "createSurface": "createSurface",
    "updateComponents": "updateComponents",
    "updateDataModel": "updateDataModel",
    "deleteSurface": "deleteSurface",
}


def _extract_a2ui_messages(msg: ToolMessage, tool_name: str, out: list) -> None:
    """Extract A2UI JSON from an MCP tool result and append to out list."""
    # Only extract A2UI JSON from render_ui tool results.
    # create_surface and other tools return partial A2UI fragments
    # that would cause "Surface already exists" errors if re-processed.
    if "render" not in tool_name.lower():
        return

    content = msg.content

    # Find the text content that contains A2UI JSON
    json_text = None
    if isinstance(content, str):
        # Single text block — try to parse directly if it looks like JSON
        stripped = content.strip()
        if stripped.startswith(("{", "[")):
            json_text = stripped
    elif isinstance(content, list):
        # MCP adapter returns list of content blocks; find JSON text blocks
        for block in content:
            if isinstance(block, dict) and block.get("type") == "text":
                candidate = block.get("text", "").strip()
                if candidate.startswith(("{", "[")):
                    json_text = candidate
                    break

    if not json_text:
        return

    try:
        parsed = json.loads(json_text)
    except (json.JSONDecodeError, TypeError):
        logger.debug("Failed to parse A2UI JSON from %s: %s", tool_name, json_text[:100])
        return

    # Extract message array from A2UI output
    messages = parsed
    if isinstance(parsed, dict) and "output" in parsed:
        messages = parsed["output"]
    if not isinstance(messages, list):
        messages = [messages]

    for m in messages:
        if not isinstance(m, dict):
            continue
        # Determine kind from message content
        kind = "unknown"
        for key, proto_kind in _A2UI_KIND_MAP.items():
            if key in m:
                kind = proto_kind
                break
        out.append(agent_pb2.A2UIMessage(kind=kind, json=json.dumps(m)))
        logger.info("Extracted A2UI message: kind=%s from tool=%s", kind, tool_name)


class AgentServicer(agent_pb2_grpc.AgentServiceServicer):
    """Implements the AgentService gRPC interface."""

    def __init__(self, config: Config) -> None:
        self.config = config
        self.sessions: dict[str, list[BaseMessage]] = {}

    async def Chat(
        self, request: agent_pb2.ChatRequest, context: grpc.ServicerContext
    ) -> agent_pb2.ChatResponse:
        """Handle a synchronous chat request."""
        try:
            return await self._chat(request, context)
        except Exception as e:
            logger.exception("Chat FAILED")
            await context.abort(grpc.StatusCode.INTERNAL, f"{type(e).__name__}: {e}")

    async def _chat(
        self, request: agent_pb2.ChatRequest, context: grpc.ServicerContext
    ) -> agent_pb2.ChatResponse:
        session_id = request.session_id or str(uuid.uuid4())
        surface_id = request.surface_id or "main"

        logger.info("=" * 60)
        logger.info("Chat request | session=%s surface=%s", session_id[:8], surface_id)
        logger.info("User: %s", request.message)

        if session_id not in self.sessions:
            self.sessions[session_id] = []

        self.sessions[session_id].append(HumanMessage(content=request.message))

        # Run agent — use persistent MCP session so all tool calls share state
        logger.info("Connecting to MCP server: %s", self.config.mcp_server_url)
        mcp_client = create_mcp_client(self.config.mcp_server_url)

        cfg = Config()
        cfg.llm_model = request.model or self.config.llm_model
        cfg.llm_api_key = self.config.llm_api_key
        cfg.llm_base_url = self.config.llm_base_url

        logger.info("Using LLM: %s @ %s", cfg.llm_model, cfg.llm_base_url)
        logger.info("Building agent graph...")

        async with mcp_client.session("a2ui") as mcp_session:
            tools = await load_mcp_tools(mcp_session)
            logger.info("Loaded %d MCP tools (persistent session)", len(tools))

            graph = await create_graph(tools, cfg)

            logger.info("Invoking agent (timeout=120s)...")
            try:
                result = await asyncio.wait_for(
                    graph.ainvoke({"messages": self.sessions[session_id]}),
                    timeout=120,
                )
            except asyncio.TimeoutError:
                logger.error("Agent invocation TIMED OUT after 120s")
                return agent_pb2.ChatResponse(
                    session_id=session_id,
                    text="Agent timed out. The LLM or MCP server may be slow.",
                    a2ui_messages=[],
                )

        messages = result["messages"]
        self.sessions[session_id] = messages

        # Extract response
        text = ""
        a2ui_messages = []

        for msg in messages:
            if isinstance(msg, AIMessage):
                if msg.content:
                    text = msg.content
                    logger.info("AI text: %s", text[:200])
                if msg.tool_calls:
                    for tc in msg.tool_calls:
                        logger.info(
                            "AI → tool_call: %s(%s)",
                            tc.get("name", "?"),
                            json.dumps(tc.get("args", {}), ensure_ascii=False)[:200],
                        )
            elif isinstance(msg, ToolMessage):
                tool_name = msg.name or msg.tool_call_id or ""
                logger.info(
                    "Tool result: %s → %s",
                    tool_name,
                    str(msg.content)[:300],
                )
                _extract_a2ui_messages(msg, tool_name, a2ui_messages)

        logger.info("Response: %d a2ui_messages, %d chars text", len(a2ui_messages), len(text))
        logger.info("=" * 60)

        return agent_pb2.ChatResponse(
            session_id=session_id,
            text=text,
            a2ui_messages=a2ui_messages,
        )

    async def ChatStream(
        self, request: agent_pb2.ChatRequest, context: grpc.ServicerContext
    ) -> AsyncIterator[agent_pb2.StreamEvent]:
        """Handle a streaming chat request."""
        try:
            async for event in self._chat_stream(request, context):
                yield event
        except Exception as e:
            logger.exception("ChatStream FAILED")
            yield agent_pb2.StreamEvent(
                error=agent_pb2.ErrorEvent(
                    code=type(e).__name__,
                    message=str(e),
                )
            )

    async def _chat_stream(
        self, request: agent_pb2.ChatRequest, context: grpc.ServicerContext
    ) -> AsyncIterator[agent_pb2.StreamEvent]:
        session_id = request.session_id or str(uuid.uuid4())
        surface_id = request.surface_id or "main"

        logger.info("=" * 60)
        logger.info("ChatStream request | session=%s surface=%s", session_id[:8], surface_id)
        logger.info("User: %s", request.message)

        # First event: session metadata
        yield agent_pb2.StreamEvent(
            session_meta=agent_pb2.SessionMeta(
                session_id=session_id,
                surface_id=surface_id,
            )
        )

        if session_id not in self.sessions:
            self.sessions[session_id] = []

        self.sessions[session_id].append(HumanMessage(content=request.message))

        logger.info("Connecting to MCP server: %s", self.config.mcp_server_url)
        mcp_client = create_mcp_client(self.config.mcp_server_url)

        cfg = Config()
        cfg.llm_model = request.model or self.config.llm_model
        cfg.llm_api_key = self.config.llm_api_key
        cfg.llm_base_url = self.config.llm_base_url

        logger.info("Using LLM: %s @ %s", cfg.llm_model, cfg.llm_base_url)

        async with mcp_client.session("a2ui") as mcp_session:
            tools = await load_mcp_tools(mcp_session)
            logger.info("Loaded %d MCP tools (persistent session)", len(tools))

            graph = await create_graph(tools, cfg)

            # Stream graph execution
            final_messages = []
            step = 0
            async for event in graph.astream_events(
                {"messages": self.sessions[session_id]},
                version="v2",
            ):
                kind = event["event"]
                data = event["data"]

                # Stream text deltas from LLM
                if kind == "on_chat_model_stream":
                    chunk = data["chunk"]
                    if hasattr(chunk, "content") and chunk.content:
                        if isinstance(chunk.content, str):
                            yield agent_pb2.StreamEvent(
                                text_delta=agent_pb2.TextDelta(text=chunk.content)
                            )

                # Report tool calls
                elif kind == "on_tool_start":
                    tool_name = event.get("name", "unknown")
                    tool_input = event.get("data", {}).get("input", {})
                    step += 1
                    logger.info(
                        "[%d] Tool call START: %s(%s)",
                        step,
                        tool_name,
                        json.dumps(tool_input, ensure_ascii=False, default=str)[:300],
                    )
                    yield agent_pb2.StreamEvent(
                        tool_call=agent_pb2.ToolCallEvent(
                            tool_name=tool_name,
                            arguments_json=json.dumps(tool_input, default=str),
                            done=False,
                        )
                    )

                elif kind == "on_tool_end":
                    tool_name = event.get("name", "unknown")
                    output = data.get("output", "")
                    output_str = output if isinstance(output, str) else str(output)
                    logger.info(
                        "[%d] Tool call END: %s → %s",
                        step,
                        tool_name,
                        output_str[:300],
                    )

                    tool_event = agent_pb2.ToolCallEvent(
                        tool_name=tool_name,
                        result=output_str,
                        done=True,
                    )

                    yield agent_pb2.StreamEvent(tool_call=tool_event)

                elif kind == "on_chain_end" and event.get("name") == "LangGraph":
                    if "output" in data:
                        final_messages = data["output"].get("messages", [])

        self.sessions[session_id] = final_messages

        # Extract A2UI messages from final tool results
        a2ui_messages = []
        for msg in final_messages:
            if isinstance(msg, ToolMessage):
                tool_name = msg.name or msg.tool_call_id or ""
                _extract_a2ui_messages(msg, tool_name, a2ui_messages)

        if a2ui_messages:
            yield agent_pb2.StreamEvent(
                a2ui_render=agent_pb2.A2UIRenderEvent(messages=a2ui_messages)
            )

        logger.info("Stream complete | %d messages, %d a2ui", len(final_messages), len(a2ui_messages))
        logger.info("=" * 60)

    async def ListModels(
        self, request, context: grpc.ServicerContext
    ) -> agent_pb2.ListModelsResponse:
        """Return available LLM models."""
        logger.info("ListModels request")
        models = [
            agent_pb2.ModelInfo(
                id=m["id"],
                display_name=m["display_name"],
                provider=m["provider"],
            )
            for m in self.config.models
        ]
        return agent_pb2.ListModelsResponse(models=models)

    async def GetSession(
        self, request: agent_pb2.GetSessionRequest, context: grpc.ServicerContext
    ) -> agent_pb2.Session:
        """Return session state."""
        session_id = request.session_id
        if session_id not in self.sessions:
            await context.abort(grpc.StatusCode.NOT_FOUND, f"Session {session_id} not found")

        messages = self.sessions[session_id]
        return agent_pb2.Session(
            session_id=session_id,
            turn_count=sum(1 for m in messages if isinstance(m, HumanMessage)),
        )

    async def DeleteSession(
        self, request: agent_pb2.DeleteSessionRequest, context: grpc.ServicerContext
    ) -> None:
        """Delete a session."""
        logger.info("DeleteSession: %s", request.session_id)
        self.sessions.pop(request.session_id, None)


async def serve(config: Config) -> None:
    """Start the gRPC server."""
    server = grpc.aio.server(futures.ThreadPoolExecutor(max_workers=10))
    agent_pb2_grpc.add_AgentServiceServicer_to_server(AgentServicer(config), server)
    addr = f"{config.grpc_host}:{config.grpc_port}"
    server.add_insecure_port(addr)
    logger.info("A2UI Agent gRPC server listening on %s", addr)
    print(f"A2UI Agent gRPC server listening on {addr}")
    await server.start()
    await server.wait_for_termination()


def main() -> None:
    # Setup logging
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)-5s %(message)s",
        datefmt="%H:%M:%S",
    )
    # Enable langchain / mcp debug logging
    logging.getLogger("langchain_mcp_adapters").setLevel(logging.DEBUG)
    logging.getLogger("mcp").setLevel(logging.DEBUG)

    # Load .env file if present
    from dotenv import load_dotenv
    env_file = Path(__file__).resolve().parent.parent / ".env"
    if env_file.exists():
        load_dotenv(env_file)
        print(f"Loaded config from {env_file}")

    config = Config.from_env()
    logger.info(
        "Config: grpc=%s:%d model=%s base_url=%s mcp=%s",
        config.grpc_host, config.grpc_port,
        config.llm_model, config.llm_base_url,
        config.mcp_server_url,
    )
    asyncio.run(serve(config))


if __name__ == "__main__":
    main()
