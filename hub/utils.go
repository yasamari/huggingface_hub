package hub

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func isSymlinkSupported(cacheDir string) (bool, error) {
	if cacheDir == "" {
		return false, nil
	}

	if symlinkSupported != nil {
		if _, ok := symlinkSupported[cacheDir]; ok {
			return true, nil
		}
	}

	fullSrcPath := filepath.Join(cacheDir, "src_test")
	file, err := os.Create(fullSrcPath)
	if err != nil {
		return false, err
	}
	defer func() {
		file.Close()
		os.Remove(fullSrcPath)
	}()

	fullDstPath := filepath.Join(cacheDir, "dst_test")
	err = os.Symlink(fullSrcPath, fullDstPath)
	if err != nil {
		return false, err
	}
	defer os.Remove(fullDstPath)

	if symlinkSupported == nil {
		symlinkSupported = make(map[string]bool)
	}

	symlinkSupported[cacheDir] = true
	return true, nil
}

func getDiskSpace(path string) (uint64, error) {

	// if runtime.GOOS != "darwin" || runtime.GOARCH != "windows" {
	// 	var stat syscall.Statfs_t
	// 	err := syscall.Statfs(path, &stat)
	// 	if err != nil {
	// 		return 0, err
	// 	}

	// 	return stat.Bavail * uint64(stat.Bsize), nil
	// } else {
	// 	// On Windows, we can use the GetDiskFreeSpaceEx and mac function to get the available space
	// 	// https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceex
	// 	var freeBytesAvailable uint64
	// 	err := syscall.GetDiskFreeSpaceEx(syscall.UTF16PtrFromString(path), nil, &freeBytesAvailable, nil, nil)
	// 	if err != nil {
	// 		return 0, err
	// 	}
	// 	return freeBytesAvailable, nil
	// }

	return 0, nil
}

func checkDiskSpace(expectedSize int, targetDir string) error {
	// targetDir = filepath.Dir(targetDir)
	// for _, path := range []string{targetDir, filepath.Dir(targetDir)} {

	// 	// Calculate available space (in bytes)
	// 	available := stat.Bavail * uint64(stat.Bsize)
	// 	if available < int64(expectedSize) {
	// 		return fmt.Errorf(
	// 			"not enough free disk space to download the file. The expected file size is: %d MB. The target location %s only has %d MB free disk space",
	// 			expectedSize/1000000, targetDir, available/1000000,
	// 		)
	// 	}
	// }

	// if _, err := os.Stat(targetDir); os.IsNotExist(err) {
	// 	return fmt.Errorf("targetDir %s does not exist", targetDir)
	// }

	return nil
}

func commonPath(path1, path2 string) string {
	dir1 := strings.Split(filepath.Clean(path1), string(filepath.Separator))
	dir2 := strings.Split(filepath.Clean(path2), string(filepath.Separator))

	var commonParts []string
	for i := 0; i < len(dir1) && i < len(dir2); i++ {
		if dir1[i] == dir2[i] {
			commonParts = append(commonParts, dir1[i])
		} else {
			break
		}
	}

	return filepath.Join(commonParts...)
}

func createSymlink(src string, dst string, newBlob bool) error {
	relativeSrc, err := filepath.Rel(filepath.Dir(dst), src)
	if err != nil {
		relativeSrc = ""
		return err
	}

	commonPath := commonPath(src, dst)
	supportSymlinks, err := isSymlinkSupported(commonPath)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			supportSymlinks, err = isSymlinkSupported(src)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if supportSymlinks {
		srcRelOrAbs := relativeSrc

		err := os.Symlink(srcRelOrAbs, dst)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				// if os.PathIsSymlink(dst) && os.Readlink(dst) == src {
				// 	// `dst` already exists and is a symlink to the `src` blob. It is most likely that the file has
				// 	// been cached twice concurrently (exactly between `os.remove` and `os.symlink`). Do nothing.
				// 	return nil
				// } else {
				// 	// Very unlikely to happen. Means a file `dst` has been created exactly between `os.remove` and
				// 	// `os.symlink` and is not a symlink to the `src` blob file. Raise exception.
				// 	return err
				// }

				return nil
			} else {
				if newBlob {
					err := os.Rename(src, dst)
					if err != nil {
						mustCopy(src, dst)
					}
				} else {
					mustCopy(src, dst)
				}
			}
		}
	}

	return nil
}

func normalizeETag(etag string) string {
	etag = strings.TrimPrefix(strings.TrimSpace(etag), "W/")
	etag = strings.Trim(etag, "\"")

	return etag
}

func mustCopy(src string, dst string) {
	srcFile, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		panic(err)
	}
}
