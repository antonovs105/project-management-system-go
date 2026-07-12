package attachment

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// ClamAVScanner streams uploads through clamd's INSTREAM protocol.
type ClamAVScanner struct{ address string }

// NewClamAVScanner returns a bounded clamd client.
func NewClamAVScanner(address string) *ClamAVScanner {
	return &ClamAVScanner{address: strings.TrimSpace(address)}
}

// Scan rejects infected or indeterminate content.
func (s *ClamAVScanner) Scan(ctx context.Context, source io.Reader) error {
	connection, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", s.address)
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := io.WriteString(connection, "zINSTREAM\x00"); err != nil {
		return err
	}
	buffer := make([]byte, 32*1024)
	for {
		count, readErr := source.Read(buffer)
		if count > 0 {
			header := make([]byte, 4)
			binary.BigEndian.PutUint32(header, uint32(count))
			if _, err := connection.Write(header); err != nil {
				return err
			}
			if _, err := connection.Write(buffer[:count]); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if _, err := connection.Write([]byte{0, 0, 0, 0}); err != nil {
		return err
	}
	response, err := bufio.NewReader(connection).ReadString(0)
	if err != nil {
		return err
	}
	response = strings.TrimSpace(strings.TrimSuffix(response, "\x00"))
	switch {
	case strings.HasSuffix(response, " FOUND"):
		return ErrInfected
	case strings.HasSuffix(response, " OK"):
		return nil
	default:
		return fmt.Errorf("clamav scan failed: %s", response)
	}
}
