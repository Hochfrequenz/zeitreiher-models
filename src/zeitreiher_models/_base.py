"""Base classes shared by all models."""

import ast
import inspect
import itertools
import textwrap
from collections.abc import Mapping
from enum import Enum, StrEnum
from typing import Any, ClassVar, NoReturn, TypeVar

from pydantic import (
    BaseModel,
    ConfigDict,
    GetJsonSchemaHandler,
    SerializerFunctionWrapHandler,
    ValidationInfo,
    model_serializer,
    model_validator,
)
from pydantic.json_schema import JsonSchemaValue
from pydantic_core import CoreSchema, PydanticCustomError

_K = TypeVar("_K")
_V = TypeVar("_V")

#: Characters that Go's encoding/json always escapes inside strings (HTML-sensitive characters and the
#: JavaScript line terminators). pydantic writes them raw; :func:`go_compatible_json` escapes them the
#: same way so that both languages write byte-for-byte identical JSON.
_GO_ESCAPES = str.maketrans({"<": "\\u003c", ">": "\\u003e", "&": "\\u0026", "\u2028": "\\u2028", "\u2029": "\\u2029"})


def go_compatible_json(text: str) -> str:
    """Escape ``<``, ``>``, ``&``, U+2028 and U+2029 as Go's ``json.Marshal`` does.

    These characters can only occur inside JSON strings, where the escaped form means the same.
    """
    return text.translate(_GO_ESCAPES)


class FrozenDict(dict[_K, _V]):
    """A read-only ``dict`` (keys sorted on creation by the models) so that frozen models are deeply immutable."""

    __slots__ = ()

    def _read_only(self, *args: Any, **kwargs: Any) -> NoReturn:
        raise TypeError(f"{type(self).__name__} is read-only")

    __setitem__ = __delitem__ = clear = pop = popitem = setdefault = update = __ior__ = _read_only

    def __hash__(self) -> int:  # type: ignore[override]
        return hash(frozenset(self.items()))

    def __reduce__(self) -> tuple[Any, ...]:
        return (type(self), (dict(self),))


def freeze_sorted(value: Mapping[str, _V]) -> FrozenDict[str, _V]:
    """Validator for JSON objects: a read-only copy with the keys sorted, as Go writes map keys."""
    return FrozenDict(sorted(value.items()))


class _Model(BaseModel):
    """Frozen model with alias-based JSON and control over which ``None`` fields are omitted.

    Scalars are strict through their annotated types (``StrictInt``, ``StrictStr``, …), not through
    ``ConfigDict(strict=True)``, so that nested models and tuples can still be built from plain
    ``dict``/``list`` values in Python.
    """

    model_config = ConfigDict(
        frozen=True,
        validate_by_name=True,
        validate_by_alias=True,
        serialize_by_alias=True,
    )

    #: JSON keys (aliases) that are left out of the output when their value is ``None``.
    #: Every other ``None`` is written as ``null``.
    _omit_if_none: ClassVar[frozenset[str]] = frozenset()

    #: True for requests: JSON keys that are Python field names instead of aliases (``from_``) are
    #: rejected. False for responses: such keys are dropped like any other unknown key.
    _reject_field_names: ClassVar[bool] = False

    @model_validator(mode="before")
    @classmethod
    def _alias_only_in_json(cls, data: Any, info: ValidationInfo) -> Any:
        """In JSON, only aliases are keys; ``{"from_": …}`` is not ``{"from": …}``."""
        if info.mode != "json" or not isinstance(data, dict):
            return data
        names = {name for name, field in cls.model_fields.items() if field.alias and field.alias != name}
        found = names.intersection(data)
        if not found:
            return data
        if cls._reject_field_names:
            raise PydanticCustomError("unknown_key", "unknown key(s) {keys}", {"keys": sorted(found)})
        return {k: v for k, v in data.items() if k not in found}

    def model_dump_json(self, **kwargs: Any) -> str:
        """Like :meth:`pydantic.BaseModel.model_dump_json`, with Go's escaping (:func:`go_compatible_json`)."""
        return go_compatible_json(super().model_dump_json(**kwargs))

    @model_serializer(mode="wrap")
    def _omit_none(self, handler: SerializerFunctionWrapHandler) -> dict[str, Any]:
        data: dict[str, Any] = handler(self)
        for key in self._omit_if_none:
            if key in data and data[key] is None:
                del data[key]
        return data


class RequestModel(_Model):
    """A request object: unknown keys are rejected."""

    model_config = ConfigDict(extra="forbid")
    _reject_field_names: ClassVar[bool] = True


class ResponseModel(_Model):
    """A response object: unknown keys are ignored so that additive server changes do not break clients."""

    model_config = ConfigDict(extra="ignore")


def member_descriptions(enum: type[Enum]) -> dict[str, str]:
    """Map each member value of ``enum`` to its attribute docstring (the string literal below the member).

    Empty when the source is not available (e.g. a frozen application).
    """
    try:
        tree = ast.parse(textwrap.dedent(inspect.getsource(enum)))
    except (OSError, TypeError, SyntaxError):
        return {}
    body = tree.body[0].body if tree.body and isinstance(tree.body[0], ast.ClassDef) else []
    descriptions: dict[str, str] = {}
    for node, following in itertools.pairwise(body):
        if (
            isinstance(node, ast.Assign)
            and len(node.targets) == 1
            and isinstance(node.targets[0], ast.Name)
            and isinstance(following, ast.Expr)
            and isinstance(following.value, ast.Constant)
            and isinstance(following.value.value, str)
        ):
            member = enum[node.targets[0].id]
            descriptions[str(member.value)] = inspect.cleandoc(following.value.value)
    return descriptions


class DocumentedStrEnum(StrEnum):
    """A ``StrEnum`` whose member docstrings are added to the JSON schema as ``x-enum-descriptions``."""

    @classmethod
    def __get_pydantic_json_schema__(cls, core_schema: CoreSchema, handler: GetJsonSchemaHandler) -> JsonSchemaValue:
        schema = handler.resolve_ref_schema(handler(core_schema))
        descriptions = member_descriptions(cls)
        if descriptions:
            schema["x-enum-descriptions"] = descriptions
        return schema
