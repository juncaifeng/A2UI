"""A2UI Agent — Proto code generation CLI.

Usage:
    uv run generate          # 生成 Python + Go stubs
    uv run generate --py     # 仅生成 Python stubs
    uv run generate --go     # 仅生成 Go stubs
    uv run generate --clean  # 清除已生成的文件
"""

from __future__ import annotations

import argparse
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
API_DIR = ROOT / "api"
GEN_PY = ROOT / "gen" / "py"
GEN_GO = ROOT / "gen" / "go"
GATEWAY_MODCACHE_PATTERN = "grpc-gateway/v2@*"


def _find_gateway_proto_dir() -> Path:
    """Locate grpc-gateway protoc includes from Go module cache."""
    gomodcache = subprocess.check_output(
        ["go", "env", "GOMODCACHE"], text=True
    ).strip()

    for p in sorted(Path(gomodcache).glob(f"github.com/grpc-ecosystem/{GATEWAY_MODCACHE_PATTERN}"), reverse=True):
        candidate = p / "protoc-gen-grpc-gateway"
        if candidate.is_dir():
            return candidate

    print("Installing grpc-gateway protoc plugin...")
    subprocess.check_call([
        "go", "install",
        "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest",
    ])
    for p in sorted(Path(gomodcache).glob(f"github.com/grpc-ecosystem/{GATEWAY_MODCACHE_PATTERN}"), reverse=True):
        candidate = p / "protoc-gen-grpc-gateway"
        if candidate.is_dir():
            return candidate

    print("ERROR: Cannot find grpc-gateway proto includes", file=sys.stderr)
    sys.exit(1)


def gen_py() -> None:
    """Generate Python gRPC stubs."""
    GEN_PY.mkdir(parents=True, exist_ok=True)

    proto_files = list(API_DIR.rglob("*.proto"))
    if not proto_files:
        print("No .proto files found under api/", file=sys.stderr)
        sys.exit(1)

    cmd = [
        sys.executable, "-m", "grpc_tools.protoc",
        f"-I{API_DIR}",
        f"--python_out={GEN_PY}",
        f"--pyi_out={GEN_PY}",
        f"--grpc_python_out={GEN_PY}",
        *[str(p) for p in proto_files],
    ]
    print(f"Generating Python stubs → {GEN_PY.relative_to(ROOT)}")
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(result.stderr, file=sys.stderr)
        sys.exit(1)
    print("  OK")


def gen_go() -> None:
    """Generate Go gRPC + grpc-gateway stubs."""
    go_module = "github.com/juncaifeng/a2ui-agent/gen/go"
    GEN_GO.mkdir(parents=True, exist_ok=True)

    gateway_dir = _find_gateway_proto_dir()

    proto_files = [
        p for p in API_DIR.rglob("*.proto")
        if not p.is_relative_to(API_DIR / "google")
    ]
    if not proto_files:
        print("No .proto files found under api/", file=sys.stderr)
        sys.exit(1)

    cmd = [
        "protoc",
        f"-I{API_DIR}",
        f"-I{gateway_dir}",
        f"--go_out={GEN_GO}",
        f"--go_opt=module={go_module}",
        f"--go-grpc_out={GEN_GO}",
        f"--go-grpc_opt=module={go_module}",
        f"--grpc-gateway_out={GEN_GO}",
        f"--grpc-gateway_opt=module={go_module}",
        *[str(p) for p in proto_files],
    ]
    print(f"Generating Go stubs → {GEN_GO.relative_to(ROOT)}")
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(result.stderr, file=sys.stderr)
        sys.exit(1)
    print("  OK")


def clean() -> None:
    """Remove all generated files."""
    for d in [GEN_PY, GEN_GO]:
        if d.exists():
            print(f"Removing {d.relative_to(ROOT)}")
            shutil.rmtree(d)
    print("Clean done.")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="A2UI Agent proto code generator",
    )
    parser.add_argument("--py", action="store_true", help="Generate Python stubs only")
    parser.add_argument("--go", action="store_true", help="Generate Go stubs only")
    parser.add_argument("--clean", action="store_true", help="Remove generated files")
    args = parser.parse_args()

    if args.clean:
        clean()
        return

    do_py = args.py or (not args.go)
    do_go = args.go or (not args.py)

    if do_py:
        gen_py()
    if do_go:
        gen_go()


if __name__ == "__main__":
    main()
