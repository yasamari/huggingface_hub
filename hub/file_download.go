package hub

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/schollz/progressbar/v3"
)

type HfFileMetadata struct {
	CommitHash string
	ETag       string
	Location   string
	Size       int
}

var symlinkSupported map[string]bool

const (
	DefaultCacheDir = "/tmp/cozy-hub-cache"
	DefaultRevision = "main"
)

const (
	ModelRepoType   = "model"
	SpaceRepoType   = "space"
	DatasetRepoType = "dataset"
)

const (
	defaultHfEndpoint        = "https://huggingface.co"
	defaultHfStagingEndpoint = "https://hub-ci.huggingface.co"
	hfUrlTemplate            = "{{.Endpoint}}/{{.RepoId}}/resolve/{{.Revision}}/{{.Filename}}"
)

const (
	HeaderFilenamePattern = `filename=\"(?P<filename>.*?)\";`
)

var IsStaging bool
var Endpoint = os.Getenv("HF_ENDPOINT")

const DownloadChunkSize = 1024 * 1024

var RepoTypes = []string{ModelRepoType, SpaceRepoType, DatasetRepoType}
var RepoTypesUrlPrefixes = map[string]string{
	SpaceRepoType:   "spaces/",
	DatasetRepoType: "datasets/",
}

func init() {
	isStaging, err := strconv.ParseBool(os.Getenv("HUGGINGFACE_CO_STAGING"))
	if err != nil {
		isStaging = false
	}

	IsStaging = isStaging
	if Endpoint == "" {
		if isStaging {
			Endpoint = defaultHfStagingEndpoint
		} else {
			Endpoint = defaultHfEndpoint
		}
	}
}

func IsSymlinkSupported(cacheDir string) bool {
	if symlinkSupported != nil {
		if _, ok := symlinkSupported[cacheDir]; ok {
			return true
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
		return false
	}

	defer file.Close()
	defer os.Remove(fullSrcPath)

	fullDstPath := filepath.Join(cacheDir, dstPath)
	err = os.Symlink(fullSrcPath, fullDstPath)
	if err != nil {
		return false
	}

	if symlinkSupported == nil {
		symlinkSupported = make(map[string]bool)
	}

	symlinkSupported[cacheDir] = true
	return true
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
	proxies map[string]string,
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

	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	contentLength, err := strconv.Atoi(r.Header.Get("Content-Length"))
	if err != nil {
		if nbRetries <= 0 {
			return fmt.Errorf("error while downloading from %s: %s\nMax retries exceeded", url, err)
		}

		if errors.Is(err, http.ErrHandlerTimeout) {
			fmt.Printf("error while downloading from %s: %s\nTrying to resume download...\n", url, err)
			time.Sleep(1 * time.Second)
			return HttpGet(url, tempFile, proxies, resumeSize, initialHeaders, expectedSize, displayedFilename, nbRetries-1, bar)
		}

		return err
	}

	// NOTE: 'total' is the total number of bytes to download, not the number of bytes in the file.
	//       If the file is compressed, the number of bytes in the saved file will be higher than 'total'.
	total := resumeSize + int64(contentLength)

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
		if _, err := r.Body.Read(buf); err != nil {
			return err
		}

		bar.Add64(int64(len(buf)))
		tempFile.Write(buf)
		newResumeSize += int64(len(buf))

		// Some data has been downloaded from the server so we reset the number of retries.
		nbRetries = 5

		if expectedSize != 0 && newResumeSize != expectedSize {
			return fmt.Errorf(consistencyErrorMessage, newResumeSize)
		}
	}
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
	params map[string]string,
) (*http.Response, error) {
	// Recursively follow relative redirects
	if followRelativeRedirects {
		response, err := requestWrapper(method, rawUrl, false, params)
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
				return requestWrapper(method, nextUrl.String(), true, params)
			}
		}
	}

	response, err := http.Get(rawUrl)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func downloadToTmpAndMove(
	incompletePath,
	destinationPath,
	urlToDownload string,
	proxies,
	headers map[string]string,
	expectedSize int,
	filename string,
	forceDownload bool,
) error {

	if _, err := os.Stat(destinationPath); err == nil && !forceDownload {
		// Do nothing if already exists (except if force_download=True)
		return nil
	}

	if _, err := os.Stat(incompletePath); err == nil && forceDownload {
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

	err = HttpGet(
		urlToDownload,
		f,
		proxies,
		resumeSize,
		headers,
		int64(expectedSize),
		filename,
		0,
		nil,
	)

	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(fmt.Sprintf("Download complete. Moving file to %s", destinationPath))
	os.Rename(incompletePath, destinationPath)

	return nil

}

func chmodAndMove(src string, dst string) error {
	tmpFile := filepath.Dir(dst) + "/tmp_" + uuid.New().String()

	file, err := os.OpenFile(tmpFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	defer os.Remove(tmpFile)

	fileInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	err = file.Chmod(fileInfo.Mode())
	if err != nil {
		return err
	}

	return os.Rename(tmpFile, dst)
}

func copyNoMatterWhat(src string, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	return nil
}

func intOrNone(value interface{}) *int {
	if value == nil {
		return nil
	}
	v, ok := value.(int)
	if !ok {
		return nil
	}
	return &v
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

func createRelativeSymlink(src string, dst string, newBlob bool) error {
	return nil
}

func createSymlink(src string, dst string, newBlob bool) error {
	return nil
}

func normalizeEtag(etag string) string {
	return etag
}
