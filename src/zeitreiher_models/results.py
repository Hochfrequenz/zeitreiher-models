"""Responses of the resolve and versions endpoints (spec §4.4)."""

from typing import Any, ClassVar, Self

from pydantic import Field, SerializerFunctionWrapHandler, model_serializer, model_validator

from zeitreiher_models._base import ResponseModel
from zeitreiher_models.errors import ResultError
from zeitreiher_models.records import Version
from zeitreiher_models.scalars import NonEmptyStr, NonNegativeInt, RawUnit, ResponseProfile, UnitCode
from zeitreiher_models.selectors import EchoedSelector
from zeitreiher_models.timestamps import ResponseInstant, check_window


class Assignment(ResponseModel):
    """A profile assignment, cut to the request window, or an assignment-level error.

    ``from``/``to`` are set together; they are required unless ``error`` is set.
    """

    _omit_if_none: ClassVar[frozenset[str]] = frozenset({"from", "to", "error"})

    profile: ResponseProfile = Field(description="The profile number without leading zeros (1 to 18 digits).")
    role: NonEmptyStr | None = Field(
        default=None,
        description="The role of the profile assignment (SAP EPROFASS role, e.g. '0001'), or null when it was "
        "not resolved through a role (e.g. by: profile). Always written (possibly null) without error.",
    )
    from_: ResponseInstant | None = Field(
        default=None,
        alias="from",
        description="Start of the assignment (inclusive), cut to the request window; RFC 3339 UTC with Z. "
        "Set together with to.",
    )
    to: ResponseInstant | None = Field(
        default=None,
        description="End of the assignment (exclusive), cut to the request window; RFC 3339 UTC with Z.",
    )
    unit: UnitCode | None = Field(
        default=None,
        description="The canonical unit code (UN/ECE Rec. 20, e.g. 'KWH') of the profile's values, or null when "
        "the SAP unit has no mapping. Always written (possibly null) without error.",
    )
    unit_raw: RawUnit | None = Field(
        default=None,
        description="The SAP unit key (T006 MSEHI) as SAP delivers it, mapped or not: 1 to 30 characters, no "
        "control characters. Required without error.",
    )
    error: ResultError | None = Field(
        default=None, description="An assignment-level error, e.g. access_denied; omitted for a usable assignment."
    )

    @model_serializer(mode="wrap")
    def _omit_none(self, handler: SerializerFunctionWrapHandler) -> dict[str, Any]:
        data: dict[str, Any] = handler(self)
        omit = set(self._omit_if_none)
        if self.error is not None:
            omit |= {"role", "unit", "unit_raw"}
        for key in omit:
            if key in data and data[key] is None:
                del data[key]
        return data

    @model_validator(mode="after")
    def _units(self) -> Self:
        if self.error is None:
            missing = [k for k in ("role", "unit") if k not in self.model_fields_set]
            if self.unit_raw is None:
                missing.append("unit_raw")
            if missing:
                raise ValueError(f"an assignment without error needs {', '.join(missing)}")
        return self

    @model_validator(mode="after")
    def _window(self) -> Self:
        if (self.from_ is None) != (self.to is None):
            raise ValueError("from and to must be set together")
        if self.from_ is None and self.error is None:
            raise ValueError("an assignment without error needs from and to")
        if self.from_ is not None and self.to is not None:
            check_window(self.from_, self.to)
        return self


class VersionedAssignment(Assignment):
    """An assignment of the versions endpoint: ``versions`` is required unless ``error`` is set."""

    _omit_if_none: ClassVar[frozenset[str]] = frozenset({"from", "to", "error", "versions"})

    versions: tuple[Version, ...] | None = Field(
        default=None,
        description="The versions overlapping the assignment; written (possibly []) unless error is set.",
    )

    @model_validator(mode="after")
    def _versions(self) -> Self:
        if self.error is None and self.versions is None:
            raise ValueError("versions is required for an assignment without error")
        if self.error is not None and self.versions is not None:
            raise ValueError("an assignment with error has no versions")
        return self


class _Result(ResponseModel):
    _omit_if_none: ClassVar[frozenset[str]] = frozenset({"assignments", "error"})

    selector: NonNegativeInt = Field(description="Index of the request selector (0-based).")
    select: EchoedSelector = Field(description="The request selector, echoed.")
    error: ResultError | None = Field(
        default=None, description="A selector-level error; exactly one of assignments or error is set."
    )

    @model_validator(mode="after")
    def _exactly_one(self) -> Self:
        if (getattr(self, "assignments", None) is None) == (self.error is None):
            raise ValueError("exactly one of assignments or error must be set")
        return self


class ResolveResult(_Result):
    """The outcome for one request selector: ``assignments`` (possibly empty) or ``error``."""

    assignments: tuple[Assignment, ...] | None = Field(
        default=None, description="The profile assignments of the selector (possibly empty); unset with error."
    )


class VersionsResult(_Result):
    """The outcome for one request selector of the versions endpoint."""

    assignments: tuple[VersionedAssignment, ...] | None = Field(
        default=None, description="The profile assignments with their versions (possibly empty); unset with error."
    )


class ResolveResponse(ResponseModel):
    """Body of a successful resolve response."""

    results: tuple[ResolveResult, ...] = Field(description="One result per request selector, in request order.")


class VersionsResponse(ResponseModel):
    """Body of a successful versions response."""

    results: tuple[VersionsResult, ...] = Field(description="One result per request selector, in request order.")
