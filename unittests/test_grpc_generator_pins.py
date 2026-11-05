"""The generator versions in scripts/generate_grpc.py equal the pins of pyproject.toml's codegen group."""

import importlib.util
import re
import tomllib
from pathlib import Path

import pytest

pytest.importorskip("grpc_tools")

ROOT = Path(__file__).resolve().parent.parent


def test_script_constants_equal_codegen_pins() -> None:
    spec = importlib.util.spec_from_file_location("generate_grpc", ROOT / "scripts" / "generate_grpc.py")
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    pins = {}
    for requirement in tomllib.loads((ROOT / "pyproject.toml").read_text(encoding="utf-8"))["dependency-groups"][
        "codegen"
    ]:
        match = re.fullmatch(r"([A-Za-z0-9_.-]+)==([^\s;]+)", requirement)
        assert match, f"codegen pins must be exact: {requirement}"
        pins[match[1]] = match[2]
    assert module.EXPECTED_VERSIONS == pins
