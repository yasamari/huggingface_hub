package hub

import (
	"os"
	"strconv"
)

type HFClient struct {
	Endpoint  string
	Token     string
	CacheDir  string
	UserAgent string
}

type HfRepo struct {
	RepoId   string
	RepoType string
	Revision string
}

type HfFile struct {
	FileName  string
	SubFolder string
	Revision  string
	Repo      *HfRepo
}

type HfFileMetadata struct {
	CommitHash string
	ETag       string
	Location   string
	Size       int
}

const (
	DefaultRevision  = "main"
	DefaultCacheDir  = "/tmp/cozy-hub-cache"
	DefaultUserAgent = "unkown/None; hf-hub/v0.0.1; rust/unknown"
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
	CommitHashPattern     = "^[0-9a-f]{40}$"
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

func NewHFClient(endpoint string, token string, cacheDir string) *HFClient {
	return &HFClient{
		Endpoint: endpoint,
		Token:    token,
		CacheDir: cacheDir,
	}
}

func DefaultClient() *HFClient {
	return &HFClient{
		Endpoint: defaultHfEndpoint,
		Token:    "",
		CacheDir: DefaultCacheDir,
	}
}

func (r *HfRepo) File(fileName string) *HfFile {
	return &HfFile{
		Repo:     r,
		FileName: fileName,
	}
}

func NewHfRepo(repoId string) *HfRepo {
	return &HfRepo{
		RepoId:   repoId,
		Revision: DefaultRevision,
	}
}

func (r *HfRepo) WithRevision(revision string) *HfRepo {
	r.Revision = revision
	return r
}

func (f *HfFile) WithSubFolder(subFolder string) *HfFile {
	f.SubFolder = subFolder
	return f
}

func (f *HfFile) WithRepo(repo *HfRepo) *HfFile {
	f.Repo = repo
	return f
}

func (client *HFClient) FileDownload(file *HfFile, forceDownload bool, localFilesOnly bool) (string, error) {
	if file.Revision == "" {
		file.Revision = DefaultRevision
	}
	if file.Repo.RepoType == "" {
		file.Repo.RepoType = ModelRepoType
	}
	if client.CacheDir == "" {
		client.CacheDir = DefaultCacheDir
	}

	return fileDownload(client, file, forceDownload, localFilesOnly)
}
