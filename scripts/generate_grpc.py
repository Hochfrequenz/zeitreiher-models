"""Regenerate the Python stubs for zeitreiher.v1 into src/zeitreiher_models/grpc/v1.

Run with `uv run --group codegen python scripts/generate_grpc.py`. The output is committed; CI reruns this
script and fails on a diff. protoc emits absolute imports of the proto package (`from zeitreiher.v1 import
...`); they are rewritten to `zeitreiher_models.grpc.v1`, so the wheel keeps a single top-level package.
"""

import os
import re
import shutil
import sys
import tempfile
from pathlib import Path

import grpc_tools
from grpc_tools import protoc

ROOT = Path(__file__).resolve().parent.parent
PROTO_DIR = ROOT / "proto"
OUT_DIR = ROOT / "src" / "zeitreiher_models" / "grpc" / "v1"
SERVICE_PROTO = "zeitreiher/v1/series_service.proto"
# .py: only the import lines (the serialized descriptors and the method paths keep `zeitreiher.v1`).
IMPORT = re.compile(r"^(from|import) zeitreiher\.v1\b", re.MULTILINE)
# .pyi (mypy-protobuf): also the qualified type references, e.g. `zeitreiher.v1.query_pb2.Record`.
QUALIFIED = re.compile(r"(?<![\w.])zeitreiher\.v1\b")
PACKAGE = "zeitreiher_models.grpc.v1"
INIT = '"""Generated protobuf and gRPC stubs for zeitreiher.v1 (scripts/generate_grpc.py). Do not edit."""\n'


def _protoc(args: list[str]) -> None:
    well_known = str(Path(grpc_tools.__file__).parent / "_proto")
    if protoc.main(["protoc", f"-I{PROTO_DIR}", f"-I{well_known}", *args]) != 0:
        sys.exit(f"protoc failed: {args}")


def main() -> None:
    """Generate, rewrite the imports and replace OUT_DIR."""
    protos = sorted(str(p.relative_to(PROTO_DIR)) for p in (PROTO_DIR / "zeitreiher" / "v1").glob("*.proto"))
    mypy = shutil.which("protoc-gen-mypy")
    mypy_grpc = shutil.which("protoc-gen-mypy_grpc")
    if mypy is None or mypy_grpc is None:
        sys.exit("mypy-protobuf plugins not found; run via `uv run --group codegen`")
    with tempfile.TemporaryDirectory() as tmp:
        _protoc([f"--python_out={tmp}", f"--plugin=protoc-gen-mypy={mypy}", f"--mypy_out={tmp}", *protos])
        # Only the service file gets gRPC stubs (the others would be empty modules).
        _protoc(
            [
                f"--grpc_python_out={tmp}",
                f"--plugin=protoc-gen-mypy_grpc={mypy_grpc}",
                f"--mypy_grpc_out={tmp}",
                SERVICE_PROTO,
            ]
        )
        generated = Path(tmp) / "zeitreiher" / "v1"
        if OUT_DIR.exists():
            shutil.rmtree(OUT_DIR)
        OUT_DIR.mkdir(parents=True)
        for src in sorted(generated.iterdir()):
            text = src.read_text(encoding="utf-8")
            text = QUALIFIED.sub(PACKAGE, text) if src.suffix == ".pyi" else IMPORT.sub(rf"\1 {PACKAGE}", text)
            (OUT_DIR / src.name).write_text(text, encoding="utf-8", newline="\n")
        (OUT_DIR / "__init__.py").write_text(INIT, encoding="utf-8", newline="\n")
    print(f"generated {len(os.listdir(OUT_DIR))} files in {OUT_DIR.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
