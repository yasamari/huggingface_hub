package hub

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/schollz/progressbar/v3"
)

var symlinkSupported map[string]bool

func IsSymlinkSupported(cacheDir string) (bool, error) {
	if symlinkSupported != nil {
		if _, ok := symlinkSupported[cacheDir]; ok {
			return true, nil
		}
	}

	srcPath := "src_test"
	dstPath := "dst_test"

	if cacheDir == "" {
		cacheDir = DefaultCacheDir
	}

	fullSrcPath := filepath.Join(cacheDir, srcPath)
	file, err := os.Create(fullSrcPath)
	if err != nil {
		return false, err
	}

	defer file.Close()
	defer os.Remove(fullSrcPath)

	fullDstPath := filepath.Join(cacheDir, dstPath)
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

func HfHubUrl(repoId string, filename string, subfolder string, repoType string, revision string, endpoint string) (string, error) {
	if subfolder != "" {
		filename = fmt.Sprintf("%s/%s", subfolder, filename)
	}

	if repoType != "" {
		repoTypePrefix, ok := RepoTypesUrlPrefixes[repoType]
		if !ok {
			return "", fmt.Errorf("invalid repo type: %s", repoType)
		}

		repoId = fmt.Sprintf("%s%s", repoTypePrefix, repoId)
	}

	if revision == "" {
		revision = DefaultRevision
	}

	url, err := formatHfUrl(
		map[string]string{
			"Endpoint": Endpoint,
			"RepoId":   repoId,
			"Revision": revision,
			"Filename": filename,
		},
	)

	if err != nil {
		return "", err
	}
	return url, err
}

func HttpGet(
	url string,
	tempFile *os.File,
	resumeSize int64,
	headers map[string]string,
	expectedSize int64,
	displayedFilename string,
	nbRetries int,
	bar *progressbar.ProgressBar,
) error {
	initialHeaders := headers
	headers = make(map[string]string)
	for k, v := range initialHeaders {
		headers[k] = v
	}

	if resumeSize > 0 {
		headers["Range"] = fmt.Sprintf("bytes=%d-", resumeSize)
	}

	r, err := requestWrapper("GET", url, true, headers)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	_, err = tempFile.Seek(resumeSize, io.SeekStart)
	if err != nil {
		return err
	}

	contentLength, err := strconv.Atoi(r.Header.Get("Content-Length"))
	if err != nil {
		if nbRetries <= 0 {
			return fmt.Errorf("error while downloading from %s: %s\nMax retries exceeded", url, err)
		}

		if errors.Is(err, http.ErrHandlerTimeout) {
			fmt.Printf("error while downloading from %s: %s\nTrying to resume download...\n", url, err)
			time.Sleep(1 * time.Second)
			return HttpGet(url, tempFile, resumeSize, initialHeaders, expectedSize, displayedFilename, nbRetries-1, bar)
		}

		return err
	}

	// NOTE: 'total' is the total number of bytes to download, not the number of bytes in the file.
	//       If the file is compressed, the number of bytes in the saved file will be higher than 'total'.
	total := resumeSize + int64(contentLength)
	if total != expectedSize {
		return fmt.Errorf("expected size %d does not match actual size %d", expectedSize, total)
	}

	if displayedFilename == "" {
		displayedFilename = url
		contentDisposition := r.Header.Get("Content-Disposition")
		if contentDisposition != "" {
			headerFilenamePattern := regexp.MustCompile(HeaderFilenamePattern)
			match := headerFilenamePattern.FindStringSubmatch(contentDisposition)
			if len(match) > 0 {
				// Means file is on CDN
				displayedFilename = match[1]
			}
		}
	}

	if len(displayedFilename) > 40 {
		displayedFilename = fmt.Sprintf("(…)%s", displayedFilename[len(displayedFilename)-40:])
	}

	consistencyErrorMessage := fmt.Sprintf("Consistency check failed: file should be of size %d but has size %d (%s). We are sorry for the inconvenience. Please retry with `force_download=True`. If the issue persists, please let us know by opening an issue on https://github.com/huggingface/huggingface_hub.", expectedSize, 0 /*/*tempFile.Tell()*/, displayedFilename)

	bar.ChangeMax64(total)
	bar.Set64(resumeSize)

	newResumeSize := resumeSize
	for {
		if nbRetries <= 0 {
			return fmt.Errorf("error while downloading from %s: %s\nMax retries exceeded", url, err)
		}

		buf := make([]byte, DownloadChunkSize)
		n, err := r.Body.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		if n > 0 {
			// Write the actual bytes read to the file
			if _, writeErr := tempFile.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("error writing to file: %v", writeErr)
			}

			bar.Add64(int64(n))
			newResumeSize += int64(n)

			// Some data has been downloaded from the server so we reset the number of retries.
			nbRetries = 5
		}
	}

	if expectedSize != 0 && newResumeSize != expectedSize {
		return fmt.Errorf(consistencyErrorMessage, newResumeSize)
	}
	return nil
}

func formatHfUrl(params map[string]string) (string, error) {
	tmpl, err := template.New("url").Parse(hfUrlTemplate)
	if err != nil {
		return "", err
	}

	var urlBytes bytes.Buffer
	if err = tmpl.Execute(&urlBytes, params); err != nil {
		return "", err
	}

	return urlBytes.String(), nil
}

func requestWrapper(
	method string,
	rawUrl string,
	followRelativeRedirects bool,
	headers map[string]string,
) (*http.Response, error) {

	// Recursively follow relative redirects

	if followRelativeRedirects {
		response, err := requestWrapper(method, rawUrl, false, headers)
		if err != nil {
			return nil, err
		}

		if response.StatusCode >= 300 && response.StatusCode <= 399 {
			parsedTarget, err := url.Parse(response.Header.Get("Location"))
			if err != nil {
				return nil, err
			}
			if parsedTarget.Host == "" {
				// This means it is a relative 'location' headers, as allowed by RFC 7231.
				// (e.g. '/path/to/resource' instead of 'http://domain.tld/path/to/resource')
				// We want to follow this relative redirect !
				//
				// Highly inspired by `resolve_redirects` from requests library.
				// See https://github.com/psf/requests/blob/main/requests/sessions.py#L159
				nextUrl := url.URL{
					Scheme:   parsedTarget.Scheme,
					Host:     parsedTarget.Host,
					Path:     parsedTarget.Path,
					RawQuery: parsedTarget.RawQuery,
				}

				return requestWrapper(method, nextUrl.String(), true, headers)
			}
		}
	}

	var (
		response *http.Response
		err      error = nil
	)

	req, err := http.NewRequest(method, rawUrl, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	response, err = client.Do(req)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func downloadToTmpAndMove(
	incompletePath,
	destinationPath,
	urlToDownload string,
	headers map[string]string,
	expectedSize int,
	filename string,
	forceDownload bool,
) error {
	if _, err := os.Stat(destinationPath); err == nil && !forceDownload {
		// Do nothing if already exists (except if force_download=True)
		return nil
	}

	_, err := os.Stat(incompletePath)
	if err == nil && forceDownload {
		// By default, we will try to resume the download if possible.
		// However, if the user has set `force_download=True` or if `hf_transfer` is enabled, then we should
		// not resume the download => delete the incomplete file.
		message := fmt.Sprintf("Removing incomplete file '%s'", incompletePath)
		if forceDownload {
			message += " (force_download=True)"
		}
		fmt.Println(message)
		os.Remove(incompletePath)
	}

	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	} else {
		if forceDownload {
			message := fmt.Sprintf("Removing incomplete file '%s'", incompletePath)
			if forceDownload {
				message += " (force_download=True)"
			}

			fmt.Println(message)
			os.Remove(incompletePath)
		}
	}

	f, err := os.OpenFile(incompletePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	defer f.Close()

	resumeSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	message := fmt.Sprintf("Downloading '%s' to '%s'", filename, incompletePath)
	if resumeSize > 0 && expectedSize != 0 {
		message += fmt.Sprintf(" (resume from %d/%d)", resumeSize, expectedSize)
	}
	fmt.Println(message)

	if expectedSize != 0 {
		// Check disk space in both tmp and destination path
		checkDiskSpace(expectedSize, incompletePath)
		checkDiskSpace(expectedSize, destinationPath)
	}

	bar := progressbar.Default(int64(expectedSize))
	err = HttpGet(
		urlToDownload,
		f,
		resumeSize,
		headers,
		int64(expectedSize),
		filename,
		5,
		bar,
	)

	if err != nil {
		fmt.Println(err)
	}

	fmt.Printf("Download complete. Moving file to %s\n", destinationPath)
	os.Rename(incompletePath, destinationPath)
	return nil
}

func copyNoMatterWhat(src string, dst string) {
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

func checkDiskSpace(expectedSize int, targetDir string) error {
	targetDir = filepath.Dir(targetDir)
	for _, path := range []string{targetDir, filepath.Dir(targetDir)} {
		fileInfo, err := os.Stat(path)
		if err != nil {
			return err
		}

		if fileInfo.Size() < int64(expectedSize) {
			return fmt.Errorf(
				"not enough free disk space to download the file. The expected file size is: %d MB. The target location %s only has %d MB free disk space",
				expectedSize/1000000, targetDir, fileInfo.Size()/1000000,
			)
		}
	}

	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("targetDir %s does not exist", targetDir)
	}

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

	supportSymlinks, err := IsSymlinkSupported(commonPath)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			supportSymlinks, err = IsSymlinkSupported(src)
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
						copyNoMatterWhat(src, dst)
					}
				} else {
					copyNoMatterWhat(src, dst)
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

func repoFolderName(repoId string, repoType string) string {
	repoParts := strings.Split(repoId, "/")
	repo := append([]string{repoType + "s"}, repoParts...)

	return strings.Join(repo, "--")
}

func cacheCommitHashForSpecificRevision(storageFolder string, revision string, commitHash string) error {
	if revision != commitHash {
		refPath := filepath.Join(storageFolder, "refs", revision)
		err := os.MkdirAll(filepath.Dir(refPath), os.ModePerm)
		if err != nil {
			panic(err)
		}

		_, err = os.Stat(refPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				os.WriteFile(refPath, []byte(commitHash), os.ModePerm)
			}

			return err
		}

		file, err := os.ReadFile(refPath)
		if err != nil {
			return err
		}
		if string(file) != commitHash {
			os.WriteFile(refPath, []byte(commitHash), os.ModePerm)
		}

		return nil
	}
	return nil

}

func fileDownload(client *HFClient, file *HfFile, forceDownload bool, localFilesOnly bool) (string, error) {
	fileName := file.FileName
	repoId := file.Repo.Id
	repoType := file.Repo.Type

	if file.SubFolder != "" {
		fileName = fmt.Sprintf("%s/%s", file.SubFolder, fileName)
	}

	if repoType != SpaceRepoType && repoType != DatasetRepoType && repoType != ModelRepoType {
		return "", fmt.Errorf("invalid repo type: %s", repoType)
	}

	repoFolderName := repoFolderName(repoId, repoType)
	// absCacheDir, err := filepath.Abs(client.CacheDir)
	// if err != nil {
	// 	return "", err
	// }

	storageFolder := filepath.Join(client.CacheDir, repoFolderName)
	err := os.MkdirAll(storageFolder, os.ModePerm)
	if err != nil {
		return "", err
	}

	snapshotPath := filepath.Join(storageFolder, "snapshots")

	revision := file.Revision
	if regexp.MustCompile(CommitHashPattern).MatchString(revision) {
		pointerPath := filepath.Join(snapshotPath, revision, fileName)
		_, err := os.Stat(pointerPath)
		if err == nil && !forceDownload {
			return pointerPath, nil
		} else {
			if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
	}

	headers := map[string]string{"User-Agent": client.UserAgent}
	params := map[string]string{
		"Endpoint": Endpoint,
		"RepoId":   repoId,
		"Revision": revision,
		"Filename": fileName,
	}
	hfUrl, err := formatHfUrl(params)
	if err != nil {
		return "", err
	}

	fileMetadata, err := getFileMetadata(hfUrl, headers)
	if fileMetadata == nil {
		return "", fmt.Errorf("error while retrieving file metadata: %s", err)
	}

	if fileMetadata.CommitHash == "" {
		url := fmt.Sprintf("https://huggingface.co/api/models/%s", repoId)

		response, err := requestWrapper("GET", url, true, headers)
		if err != nil {
			return "", err
		}
		defer response.Body.Close()

		var data map[string]interface{}
		err = json.NewDecoder(response.Body).Decode(&data)
		if err != nil {
			return "", err
		}

		fileMetadata.CommitHash = data["sha"].(string)

		fmt.Println("No commit hash found for this file. It is likely that the file is not yet available on HF Hub.")
	}

	if fileMetadata.ETag == "" {
		fmt.Println("No ETag found for this file. It is likely that the file is not yet available on HF Hub.")
	}

	if fileMetadata.Size == 0 {
		fmt.Println("No size found for this file. It is likely that the file is not yet available on HF Hub.")
	}

	if fileMetadata.Location != hfUrl {
		parsedLocation, err := url.Parse(fileMetadata.Location)
		if err != nil {
			return "", err
		}

		parsedHfUrl, err := url.Parse(hfUrl)
		if err != nil {
			return "", err
		}

		if parsedLocation.Host != parsedHfUrl.Host {
			// Remove authorization header when downloading a LFS blob
			delete(headers, "authorization")
		}
	}

	if !(localFilesOnly || fileMetadata.ETag != "") {
		return "", fmt.Errorf("error while retrieving file metadata: %s", err)
	}

	var commitHash string
	if fileMetadata.CommitHash != "" {
		if regexp.MustCompile(CommitHashPattern).MatchString(fileMetadata.CommitHash) {
			commitHash = fileMetadata.CommitHash
		} else {
			refPath := filepath.Join(storageFolder, "refs", revision)
			_, err := os.Stat(refPath)
			if err == nil {
				content, err := os.ReadFile(refPath)
				if err != nil {
					return "", err
				}
				commitHash = string(content)
			}

			if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
	}

	if commitHash != "" {
		pointerPath := filepath.Join(snapshotPath, commitHash, fileName)
		_, err := os.Stat(pointerPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		if err == nil && !forceDownload {
			return pointerPath, nil
		}
	}

	blobPath := filepath.Join(storageFolder, "blobs", fileMetadata.ETag)
	pointerPath := filepath.Join(snapshotPath, commitHash, fileName)

	os.MkdirAll(filepath.Dir(blobPath), os.ModePerm)
	os.MkdirAll(filepath.Dir(pointerPath), os.ModePerm)

	cacheCommitHashForSpecificRevision(storageFolder, revision, commitHash)

	if !forceDownload {
		if _, err := os.Stat(pointerPath); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		} else {
			return pointerPath, nil
		}

		_, err := os.Stat(blobPath)
		if err == nil {
			createSymlink(blobPath, pointerPath, false)
			return pointerPath, nil
		} else {
			if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
	}

	incompletePath := fmt.Sprintf("%s.incomplete", blobPath)
	destinationPath := blobPath
	// destinationPath := filepath.Join(cacheDir, repoFolderName, fileName)

	err = downloadToTmpAndMove(incompletePath, destinationPath, hfUrl, headers, fileMetadata.Size, fileName, forceDownload)
	if err != nil {
		return "", err
	}

	createSymlink(destinationPath, pointerPath, true)
	return pointerPath, nil
}

func getFileMetadata(url string, headers map[string]string) (*HfFileMetadata, error) {
	headers["User-Agent"] = DefaultUserAgent
	headers["Accept-Encoding"] = "identity"

	response, err := requestWrapper("HEAD", url, true, headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	commitHash := response.Header.Get("X-Repo-Commit")

	fmt.Println("commitHash>>", commitHash)

	etag := response.Header.Get("X-Linked-Etag")
	if etag == "" {
		etag = response.Header.Get("ETag")
	}

	fmt.Println("etag>>", etag)

	sizeStr := response.Header.Get("X-Linked-Size")
	if sizeStr == "" {
		sizeStr = response.Header.Get("Content-Length")
	}

	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return nil, err
	}

	location := response.Header.Get("Location")
	if location == "" {
		location = response.Request.URL.String()
	}

	return &HfFileMetadata{
		Size:       size,
		Location:   location,
		CommitHash: commitHash,
		ETag:       normalizeETag(etag),
	}, nil
}
