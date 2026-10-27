"""Responses of the resolve and versions endpoints (spec §4.4)."""

from typing import ClassVar, Self

from pydantic import Field, model_validator

from zeitreiher_models._base import ResponseModel
from zeitreiher_models.errors import ResultError
from zeitreiher_models.records import Version
from zeitreiher_models.scalars import NonNegativeInt, ResponseProfile
from zeitreiher_models.selectors import EchoedSelector
from zeitreiher_models.timestamps import ResponseInstant, check_window


class Assignment(ResponseModel):
    """A profile assignment, cut to the request window, or an assignment-level error.

    ``from``/``to`` are set together; they are required unless ``error`` is set.
    """

    _omit_if_none: ClassVar[frozenset[str]] = frozenset({"from", "to", "error"})

    profile: ResponseProfile
    from_: ResponseInstant | None = Field(default=None, alias="from")
    to: ResponseInstant | None = None
    error: ResultError | None = None

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

    versions: tuple[Version, ...] | None = None

    @model_validator(mode="after")
    def _versions(self) -> Self:
        if self.error is None and self.versions is None:
            raise ValueError("versions is required for an assignment without error")
        if self.error is not None and self.versions is not None:
            raise ValueError("an assignment with error has no versions")
        return self


class _Result(ResponseModel):
    _omit_if_none: ClassVar[frozenset[str]] = frozenset({"assignments", "error"})

    selector: NonNegativeInt
    select: EchoedSelector
    error: ResultError | None = None

    @model_validator(mode="after")
    def _exactly_one(self) -> Self:
        if (getattr(self, "assignments", None) is None) == (self.error is None):
            raise ValueError("exactly one of assignments or error must be set")
        return self


class ResolveResult(_Result):
    """The outcome for one request selector: ``assignments`` (possibly empty) or ``error``."""

    assignments: tuple[Assignment, ...] | None = None


class VersionsResult(_Result):
    """The outcome for one request selector of the versions endpoint."""

    assignments: tuple[VersionedAssignment, ...] | None = None


class ResolveResponse(ResponseModel):
    """Body of a successful resolve response."""

    results: tuple[ResolveResult, ...]


class VersionsResponse(ResponseModel):
    """Body of a successful versions response."""

    results: tuple[VersionsResult, ...]
