"""Reading and writing NDJSON query responses."""

from collections.abc import Iterable, Iterator

from pydantic import TypeAdapter

from zeitreiher_models.records import EndRecord, Record

RECORD_ADAPTER: TypeAdapter[Record] = TypeAdapter(Record)
"""The pydantic adapter that parses and writes one NDJSON line as a :data:`~zeitreiher_models.Record`."""


class MissingEndError(ValueError):
    """The response does not end with exactly one end record: it was cut off and must be treated as failed."""


def parse_record(line: str | bytes) -> Record:
    """Parse one NDJSON line (without its newline) into a record."""
    return RECORD_ADAPTER.validate_json(line)


def dump_record(record: Record) -> str:
    """Serialize a record to one JSON line without the trailing newline.

    The record serializes itself (not through the ``Record`` union), so that a refused value, such as an
    unknown error code, raises its own error instead of a union-mismatch warning.
    """
    return record.model_dump_json()


def iter_records(lines: Iterable[str | bytes]) -> Iterator[Record]:
    r"""Yield the records of NDJSON lines, e.g. ``open(path, newline="")`` or ``response.iter_lines()``.

    Each line may end with ``\n``; a carriage return or a blank line is an error.
    """
    for number, line in enumerate(lines, start=1):
        text = line.decode("utf-8") if isinstance(line, bytes) else line
        text = text.removesuffix("\n")
        if "\r" in text:
            raise ValueError(f"line {number}: carriage return in NDJSON line (lines end with \\n only)")
        if not text.strip():
            raise ValueError(f"line {number}: blank line")
        try:
            yield parse_record(text)
        except ValueError as exc:
            raise ValueError(f"line {number}: {exc}") from exc


def split_lines(data: str | bytes) -> list[str]:
    """Split a complete NDJSON document into lines; the final newline is optional."""
    text = data.decode("utf-8") if isinstance(data, bytes) else data
    if not text:
        return []
    return text.removesuffix("\n").split("\n")


def read_stream(data: str | bytes) -> list[Record]:
    """Parse a complete response and check that it ends with exactly one end record."""
    records: list[Record] = []
    for record in iter_records(split_lines(data)):
        if records and isinstance(records[-1], EndRecord):
            raise MissingEndError("record after the end record")
        records.append(record)
    if not records or not isinstance(records[-1], EndRecord):
        raise MissingEndError("response stream does not end with an end record")
    return records
