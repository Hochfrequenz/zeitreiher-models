"""Selectors: which business object's profile assignments a request is about."""

from collections.abc import Mapping
from typing import Annotated, Any, Literal

from pydantic import AfterValidator, Discriminator, Field, StrictStr, StringConstraints, Tag, field_validator

from zeitreiher_models._base import RequestModel, ResponseModel, freeze_sorted
from zeitreiher_models.scalars import CHECK_TEXT, MaLo, RequestProfile, ResponseProfile, Text

KNOWN_SELECTOR_KINDS = frozenset({"profile", "malo"})
"""Selector kinds every installation knows; any other ``by`` is a custom (customer) resolver."""

CustomKind = Annotated[StrictStr, StringConstraints(pattern=r"^[a-z][a-z0-9_]*$")]
"""The name of a customer resolver (``by``): snake_case, starting with a lower-case letter."""

ParamKey = Annotated[StrictStr, StringConstraints(min_length=1), CHECK_TEXT]
"""The key of a custom selector parameter: a non-empty string."""


class ProfileSelector(RequestModel):
    """``{"by":"profile","profile":"50000000000056"}`` — a profile number (leading zeros allowed)."""

    by: Literal["profile"] = "profile"
    profile: RequestProfile = Field(description="Profile number, 1 to 18 digits, leading zeros allowed, not all zeros.")


class MaLoSelector(RequestModel):
    """``{"by":"malo","malo":"87253749908"}`` — the profiles assigned to a market location."""

    by: Literal["malo"] = "malo"
    malo: MaLo = Field(description="The market location id: exactly 11 digits.")


class CustomSelector(RequestModel):
    """``{"by":"<name>","params":{"<key>":"<string>"}}`` — a customer resolver."""

    by: CustomKind = Field(description="The resolver name in snake_case; not 'profile' or 'malo'.")
    params: Annotated[Mapping[ParamKey, Text], AfterValidator(freeze_sorted)] = Field(
        description="String parameters of the resolver; keys are non-empty."
    )

    @field_validator("by")
    @classmethod
    def _not_a_known_kind(cls, value: str) -> str:
        if value in KNOWN_SELECTOR_KINDS:
            raise ValueError(f"{value!r} is a built-in selector kind, not a custom one")
        return value


def _selector_tag(value: Any) -> str:
    by = value.get("by") if isinstance(value, dict) else getattr(value, "by", None)
    # A non-string (possibly unhashable) "by" goes to the custom variant, whose CustomKind rejects it.
    return by if isinstance(by, str) and by in KNOWN_SELECTOR_KINDS else "custom"


Selector = Annotated[
    Annotated[ProfileSelector, Tag("profile")]
    | Annotated[MaLoSelector, Tag("malo")]
    | Annotated[CustomSelector, Tag("custom")],
    Discriminator(_selector_tag),
]
"""A request selector: a union over ``by``."""


class EchoedProfileSelector(ResponseModel):
    """A profile selector as echoed in a response: the profile number has no leading zeros."""

    by: Literal["profile"] = "profile"
    profile: ResponseProfile = Field(description="The profile number without leading zeros.")


class EchoedMaLoSelector(ResponseModel):
    """A MaLo selector as echoed in a response."""

    by: Literal["malo"] = "malo"
    malo: MaLo = Field(description="The market location id: exactly 11 digits.")


class EchoedCustomSelector(ResponseModel):
    """A custom selector as echoed in a response."""

    by: CustomKind = Field(description="The resolver name in snake_case; not 'profile' or 'malo'.")
    params: Annotated[Mapping[ParamKey, Text], AfterValidator(freeze_sorted)] = Field(
        description="String parameters of the resolver; keys are non-empty."
    )

    @field_validator("by")
    @classmethod
    def _not_a_known_kind(cls, value: str) -> str:
        if value in KNOWN_SELECTOR_KINDS:
            raise ValueError(f"{value!r} is a built-in selector kind, not a custom one")
        return value


EchoedSelector = Annotated[
    Annotated[EchoedProfileSelector, Tag("profile")]
    | Annotated[EchoedMaLoSelector, Tag("malo")]
    | Annotated[EchoedCustomSelector, Tag("custom")],
    Discriminator(_selector_tag),
]
"""The request selector echoed in responses (``select``): unknown keys ignored, profile normalized."""
