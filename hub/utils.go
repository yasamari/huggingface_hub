package hub

import (
	"errors"
	"net"
	"os"
)

func isOfflineError(err error) bool {
	if err == nil {
		return false
	}

	var netOpErr *net.OpError
	var dnsErr *net.DNSError
	var syscallErr *os.SyscallError

	// Check if the error is a network operation error
	if errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}

	// Check if the error is a network operation error
	if errors.As(err, &netOpErr) {
		// Typically, these errors occur due to offline or unreachable network
		if netOpErr.Op == "dial" || netOpErr.Op == "read" || netOpErr.Op == "write" {
			return true
		}
	}

	// Check if the error is a DNS error
	if errors.As(err, &dnsErr) {
		return true
	}

	// Check if the error is a syscall error
	if errors.As(err, &syscallErr) {
		if syscallErr.Err == os.ErrNotExist || syscallErr.Err == os.ErrPermission {
			return false
		}
		return true
	}

	return false
}
