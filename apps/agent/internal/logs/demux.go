package logs

import (
	"bufio"
	"encoding/binary"
	"io"
	"strings"
	"time"
)

// DemuxStream reads from a Docker log reader, demultiplexes stdout/stderr headers,
// parses timestamps, applies redaction, and sends entries to the emit function.
func DemuxStream(r io.Reader, redactor *Redactor, timestamps bool, emit func(entry LogEntry)) error {
	header := make([]byte, 8)

	for {
		// Read 8-byte header
		_, err := io.ReadFull(r, header)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}

		streamType := StreamStdout
		switch header[0] {
		case 1:
			streamType = StreamStdout
		case 2:
			streamType = StreamStderr
		default:
			// Non-multiplexed raw stream fallback (e.g. TTY enabled)
			return scanRawStream(r, header, redactor, timestamps, emit)
		}

		size := binary.BigEndian.Uint32(header[4:8])
		if size == 0 {
			continue
		}

		payload := make([]byte, size)
		_, err = io.ReadFull(r, payload)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}

		// Payload may contain multiple newline-separated lines
		lines := strings.Split(string(payload), "\n")
		for _, line := range lines {
			line = strings.TrimRight(line, "\r")
			if line == "" && len(lines) > 1 {
				continue
			}

			entry := parseLogLine(streamType, line, redactor, timestamps)
			emit(entry)
		}
	}
}

// scanRawStream handles containers created with TTY where 8-byte headers are absent.
func scanRawStream(r io.Reader, initialHeader []byte, redactor *Redactor, timestamps bool, emit func(entry LogEntry)) error {
	combined := io.MultiReader(strings.NewReader(string(initialHeader)), r)
	scanner := bufio.NewScanner(combined)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		entry := parseLogLine(StreamStdout, line, redactor, timestamps)
		emit(entry)
	}
	return scanner.Err()
}

// parseLogLine parses optional RFC3339 timestamps, redacts sensitive text, and produces LogEntry.
func parseLogLine(stream StreamType, line string, redactor *Redactor, expectTimestamps bool) LogEntry {
	ts := time.Now().UTC()
	msg := line

	if expectTimestamps && len(line) > 30 {
		// Format: 2026-09-20T17:45:00.123456789Z <message>
		idx := strings.IndexByte(line, ' ')
		if idx > 0 {
			rawTS := line[:idx]
			if parsed, err := time.Parse(time.RFC3339Nano, rawTS); err == nil {
				ts = parsed.UTC()
				msg = line[idx+1:]
			}
		}
	}

	if redactor != nil {
		msg = redactor.Redact(msg)
	}

	return LogEntry{
		Stream:    stream,
		Message:   msg,
		Timestamp: ts,
	}
}
