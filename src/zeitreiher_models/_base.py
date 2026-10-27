"""Base classes shared by all models."""

from typing import Any, ClassVar

from pydantic import (
    BaseModel,
    ConfigDict,
    SerializerFunctionWrapHandler,
    ValidationInfo,
    model_serializer,
    model_validator,
)
from pydantic_core import PydanticCustomError


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
