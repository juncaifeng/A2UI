"""Agent service configuration — loaded from env vars or defaults."""

from __future__ import annotations

import os
from dataclasses import dataclass, field


@dataclass
class Config:
    # gRPC server
    grpc_host: str = "0.0.0.0"
    grpc_port: int = 50051

    # LLM provider
    llm_model: str = "deepseek-chat"
    llm_base_url: str = "https://api.deepseek.com"
    llm_api_key: str = ""

    # A2UI MCP server
    mcp_server_url: str = "http://localhost:8080/mcp"

    # Available models for ListModels
    models: list[dict[str, str]] = field(default_factory=lambda: [
        {"id": "deepseek-chat", "display_name": "DeepSeek V3", "provider": "deepseek"},
        {"id": "deepseek-reasoner", "display_name": "DeepSeek R1", "provider": "deepseek"},
        {"id": "gpt-4o", "display_name": "GPT-4o", "provider": "openai"},
    ])

    @staticmethod
    def from_env() -> Config:
        cfg = Config()
        cfg.grpc_host = os.getenv("AGENT_GRPC_HOST", cfg.grpc_host)
        cfg.grpc_port = int(os.getenv("AGENT_GRPC_PORT", cfg.grpc_port))
        cfg.llm_model = os.getenv("LLM_MODEL", cfg.llm_model)
        cfg.llm_base_url = os.getenv("LLM_BASE_URL", cfg.llm_base_url)
        cfg.llm_api_key = os.getenv("LLM_API_KEY", cfg.llm_api_key)
        cfg.mcp_server_url = os.getenv("MCP_SERVER_URL", cfg.mcp_server_url)
        return cfg
