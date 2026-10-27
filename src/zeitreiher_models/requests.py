"""Request bodies."""

from collections.abc import Mapping
from typing import Annotated, Any, ClassVar, Self

from pydantic import AfterValidator, Discriminator, Field, Tag, field_validator, model_validator

from zeitreiher_models._base import FrozenDict, RequestModel, freeze_sorted
from zeitreiher_models.scalars import Interval, PositiveInt, Token, Unit
from zeitreiher_models.selectors import Selector
from zeitreiher_models.timestamps import RequestInstant, check_window


class CurrentVersion(RequestModel):
    """``{"current": {}}`` — the current values (the default)."""

    current: Annotated[Mapping[str, Any], AfterValidator(freeze_sorted)] = Field(default_factory=FrozenDict)

    @field_validator("current")
    @classmethod
    def _empty(cls, value: Mapping[str, Any]) -> Mapping[str, Any]:
        if value:
            raise ValueError("current must be an empty object {}")
        return value


class VersionNumber(RequestModel):
    """``{"number": 3}`` — the values of version 3, only within its covered range."""

    number: PositiveInt


class VersionAsOf(RequestModel):
    """``{"as_of": "…"}`` — the values as they were known at that instant."""

    as_of: RequestInstant


def _version_tag(value: Any) -> str | None:
    if isinstance(value, dict):
        if len(value) != 1:
            return None
        return next(iter(value)) if next(iter(value)) in ("current", "number", "as_of") else None
    for tag, cls in (("current", CurrentVersion), ("number", VersionNumber), ("as_of", VersionAsOf)):
        if isinstance(value, cls):
            return tag
    return None


#: Exactly one of ``current``, ``number`` or ``as_of``.
VersionSelector = Annotated[
    Annotated[CurrentVersion, Tag("current")]
    | Annotated[VersionNumber, Tag("number")]
    | Annotated[VersionAsOf, Tag("as_of")],
    Discriminator(
        _version_tag,
        custom_error_type="version_selector",
        custom_error_message="exactly one of current, number or as_of must be set",
    ),
]

_SelectList = Annotated[tuple[Selector, ...], Field(min_length=1)]


class _WindowRequest(RequestModel):
    select: _SelectList
    from_: RequestInstant = Field(alias="from")
    to: RequestInstant

    @model_validator(mode="after")
    def _window(self) -> Self:
        check_window(self.from_, self.to)
        return self


class QueryRequest(_WindowRequest):
    """Body of ``POST /api/v1/series/query`` (spec §4.1). Optional fields that are ``None`` are omitted."""

    _omit_if_none: ClassVar[frozenset[str]] = frozenset({"version", "interval", "unit", "cursor"})

    version: VersionSelector | None = None
    interval: Interval | None = None
    unit: Unit | None = None
    cursor: Token | None = None


class ResolveRequest(_WindowRequest):
    """Body of ``POST /api/v1/series/resolve``: selectors and a window, nothing else."""


class VersionsRequest(_WindowRequest):
    """Body of ``POST /api/v1/series/versions``: selectors and a window, nothing else."""
