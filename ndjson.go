package zeitreihermodels

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
)

// ErrMissingEnd is returned by ReadStream when a response does not end with exactly one end record.
// Such a response was cut off and must be treated as a failure (spec §4.3).
var ErrMissingEnd = errors.New("response stream does not end with an end record")

// ParseRecord parses one NDJSON line (without its trailing newline) into a record.
func ParseRecord(line []byte) (Record, error) {
	head, err := checkObject(line, rules{"type": required}, ignoreUnknown)
	if err != nil {
		return nil, fmt.Errorf("record: %w", err)
	}
	var t string
	if err := json.Unmarshal(head["type"], &t); err != nil {
		return nil, errors.New("record: type must be a string")
	}
	var rec Record
	switch t {
	case RecordTypeSeries:
		rec = new(SeriesRecord)
	case RecordTypeValue:
		rec = new(ValueRecord)
	case RecordTypeError:
		rec = new(ErrorRecord)
	case RecordTypeEnd:
		rec = new(EndRecord)
	default:
		return nil, fmt.Errorf("record: unknown type %q", t)
	}
	if err := json.Unmarshal(line, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// MarshalRecord validates rec and returns its JSON form without a trailing newline.
func MarshalRecord(rec Record) ([]byte, error) {
	if rec == nil {
		return nil, errors.New("record is nil")
	}
	return json.Marshal(rec)
}

// WriteRecord writes rec as one NDJSON line.
func WriteRecord(w io.Writer, rec Record) error {
	b, err := MarshalRecord(rec)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// ReadRecords yields the records of an NDJSON stream. Every line must hold one record; the final
// newline is optional. Blank lines and carriage returns are errors. Iteration stops after the
// first error. ReadRecords does not check for the end record; use ReadStream for complete responses.
func ReadRecords(r io.Reader) iter.Seq2[Record, error] {
	return func(yield func(Record, error) bool) {
		br := bufio.NewReader(r)
		for lineNo := 1; ; lineNo++ {
			line, err := br.ReadBytes('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				yield(nil, err)
				return
			}
			if len(line) == 0 && errors.Is(err, io.EOF) {
				return
			}
			line = bytes.TrimSuffix(line, []byte{'\n'})
			rec, perr := parseLine(line)
			if perr != nil {
				yield(nil, fmt.Errorf("line %d: %w", lineNo, perr))
				return
			}
			if !yield(rec, nil) || err != nil {
				return
			}
		}
	}
}

// parseLine parses one line after framing checks.
func parseLine(line []byte) (Record, error) {
	if bytes.IndexByte(line, '\r') >= 0 {
		return nil, errors.New("carriage return in NDJSON line (lines end with \\n only)")
	}
	if len(bytes.TrimSpace(line)) == 0 {
		return nil, errors.New("blank line")
	}
	return ParseRecord(line)
}

// ReadStream reads a complete response and checks that its last record, and only its last record,
// is an end record. It returns ErrMissingEnd (wrapped) otherwise.
func ReadStream(r io.Reader) ([]Record, error) {
	var out []Record
	for rec, err := range ReadRecords(r) {
		if err != nil {
			return out, err
		}
		if len(out) > 0 && out[len(out)-1].RecordType() == RecordTypeEnd {
			return out, fmt.Errorf("%w: record after the end record", ErrMissingEnd)
		}
		out = append(out, rec)
	}
	if len(out) == 0 || out[len(out)-1].RecordType() != RecordTypeEnd {
		return out, ErrMissingEnd
	}
	return out, nil
}
