"""Selectors: which business object's profile assignments a request is about."""

from typing import Annotated, Any, Literal

from pydantic import Discriminator, StrictStr, StringConstraints, Tag, field_validator

from zeitreiher_models._base import RequestModel, ResponseModel
from zeitreiher_models.scalars import MaLo, RequestProfile, ResponseProfile

#: Selector kinds every installation knows; any other ``by`` is a custom (customer) resolver.
KNOWN_SELECTOR_KINDS = frozenset({"profile", "malo"})

CustomKind = Annotated[StrictStr, StringConstraints(pattern=r"^[a-z][a-z0-9_]*$")]
ParamKey = Annotated[StrictStr, StringConstraints(min_length=1)]


class ProfileSelector(RequestModel):
    """``{"by":"profile","profile":"50000000000056"}`` — a profile number (leading zeros allowed)."""

    by: Literal["profile"] = "profile"
    profile: RequestProfile


class MaLoSelector(RequestModel):
    """``{"by":"malo","malo":"87253749908"}`` — the profiles assigned to a market location."""

    by: Literal["malo"] = "malo"
    malo: MaLo


class CustomSelector(RequestModel):
    """``{"by":"<name>","params":{"<key>":"<string>"}}`` — a customer resolver."""

    by: CustomKind
    params: dict[ParamKey, StrictStr]

    @field_validator("by")
    @classmethod
    def _not_a_known_kind(cls, value: str) -> str:
        if value in KNOWN_SELECTOR_KINDS:
            raise ValueError(f"{value!r} is a built-in selector kind, not a custom one")
        return value


def _selector_tag(value: Any) -> str:
    by = value.get("by") if isinstance(value, dict) else getattr(value, "by", None)
    return by if by in KNOWN_SELECTOR_KINDS else "custom"


#: A request selector: a union over ``by``.
Selector = Annotated[
    Annotated[ProfileSelector, Tag("profile")]
    | Annotated[MaLoSelector, Tag("malo")]
    | Annotated[CustomSelector, Tag("custom")],
    Discriminator(_selector_tag),
]


class EchoedProfileSelector(ResponseModel):
    """A profile selector as echoed in a response: the profile number has no leading zeros."""

    by: Literal["profile"] = "profile"
    profile: ResponseProfile


class EchoedMaLoSelector(ResponseModel):
    """A MaLo selector as echoed in a response."""

    by: Literal["malo"] = "malo"
    malo: MaLo


class EchoedCustomSelector(ResponseModel):
    """A custom selector as echoed in a response."""

    by: CustomKind
    params: dict[ParamKey, StrictStr]

    @field_validator("by")
    @classmethod
    def _not_a_known_kind(cls, value: str) -> str:
        if value in KNOWN_SELECTOR_KINDS:
            raise ValueError(f"{value!r} is a built-in selector kind, not a custom one")
        return value


#: The request selector echoed in responses (``select``): unknown keys ignored, profile normalized.
EchoedSelector = Annotated[
    Annotated[EchoedProfileSelector, Tag("profile")]
    | Annotated[EchoedMaLoSelector, Tag("malo")]
    | Annotated[EchoedCustomSelector, Tag("custom")],
    Discriminator(_selector_tag),
]
