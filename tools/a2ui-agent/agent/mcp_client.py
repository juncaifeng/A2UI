"""MCP client wrapper — connects to the A2UI MCP server via HTTP transport."""

from __future__ import annotations

import sys
from pathlib import Path

# Add generated stubs to import path
_gen_py = str(Path(__file__).resolve().parent.parent / "gen" / "py")
if _gen_py not in sys.path:
    sys.path.insert(0, _gen_py)

from langchain_mcp_adapters.client import MultiServerMCPClient


def create_mcp_client(mcp_server_url: str) -> MultiServerMCPClient:
    """Create an MCP client connected to the A2UI MCP server.

    Args:
        mcp_server_url: URL of the MCP server's StreamableHTTP endpoint.

    Returns:
        A MultiServerMCPClient ready to get tools.
    """
    return MultiServerMCPClient(
        {
            "a2ui": {
                "url": mcp_server_url,
                "transport": "http",
            }
        }
    )
