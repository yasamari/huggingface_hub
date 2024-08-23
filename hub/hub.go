package hub

import (
	"fmt"
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
	Id       string
	Type     string
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

type HFModelInfo struct {
	Sha      string               `json:"sha"`
	Siblings []HFModelInfoSibling `json:"siblings"`
}

type HFModelInfoSibling struct {
	RFileName string `json:"rfilename"`
}

const (
	DefaultRevision  = "main"
	DefaultCacheDir  = "/tmp/cozy-hub-cache"
	DefaultUserAgent = "unknown/None; hf-hub/0.0.1"
)

const (
	ModelRepoType   = "model"
	SpaceRepoType   = "space"
	DatasetRepoType = "dataset"
)

const (
	defaultHfEndpoint           = "https://huggingface.co"
	defaultHfStagingEndpoint    = "https://hub-ci.huggingface.co"
	hfResolveUrlTemplate        = "{{.Endpoint}}/{{.RepoId}}/resolve/{{.Revision}}/{{.Filename}}"
	hfModelInfoTemplate         = "{{.Endpoint}}/api/models/{{.RepoId}}"
	hfModelRevisionInfoTemplate = "{{.Endpoint}}/api/models/{{.RepoId}}/revision/{{.Revision}}"
)

const (
	HeaderFilenamePattern = `filename=\"(?P<filename>.*?)\";`
	CommitHashPattern     = "^[0-9a-f]{40}$"
)

var IsStaging bool
var HFEndpoint = os.Getenv("HF_ENDPOINT")

const DownloadChunkSize = 1024 * 1024
const DefaultRetries = 5

var RepoTypes = []string{ModelRepoType, SpaceRepoType, DatasetRepoType}
var RepoTypesUrlPrefixes = map[string]string{
	SpaceRepoType:   "spaces/",
	DatasetRepoType: "datasets/",
}

var symlinkSupported map[string]bool

func init() {
	isStaging, err := strconv.ParseBool(os.Getenv("HUGGINGFACE_CO_STAGING"))
	if err != nil {
		isStaging = false
	}

	IsStaging = isStaging
	if HFEndpoint == "" {
		if isStaging {
			HFEndpoint = defaultHfStagingEndpoint
		} else {
			HFEndpoint = defaultHfEndpoint
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
		Endpoint:  HFEndpoint,
		CacheDir:  DefaultCacheDir,
		UserAgent: DefaultUserAgent,
	}
}

func (r *HfRepo) File(fileName string) *HfFile {
	return &HfFile{
		Repo:     r,
		FileName: fileName,
		Revision: r.Revision,
	}
}

func NewHfRepo(repoId string) *HfRepo {
	return &HfRepo{
		Id:       repoId,
		Type:     ModelRepoType,
		Revision: DefaultRevision,
	}
}

func (r *HfRepo) WithRevision(revision string) *HfRepo {
	r.Revision = revision
	return r
}

func (r *HfRepo) WithType(repoType string) *HfRepo {
	r.Type = repoType
	return r
}

func (f *HfFile) WithSubFolder(subFolder string) *HfFile {
	f.SubFolder = subFolder
	return f
}

func (f *HfFile) WithRepo(repo *HfRepo) *HfFile {
	f.Repo = repo
	f.Revision = repo.Revision
	return f
}

func (client *HFClient) WithCacheDir(cacheDir string) *HFClient {
	client.CacheDir = cacheDir
	return client
}

func (client *HFClient) WithEndpoint(endpoint string) *HFClient {
	client.Endpoint = endpoint
	return client
}

func (client *HFClient) WithToken(token string) *HFClient {
	client.Token = token
	return client
}

func (client *HFClient) WithUserAgent(userAgent string) *HFClient {
	client.UserAgent = userAgent
	return client
}

func (client *HFClient) FileDownload(file *HfFile, forceDownload bool, localFilesOnly bool) (string, error) {
	if file.Revision == "" {
		file.Revision = DefaultRevision
	}
	if file.Repo.Type == "" {
		file.Repo.Type = ModelRepoType
	}
	if client.CacheDir == "" {
		client.CacheDir = DefaultCacheDir
	}

	return fileDownload(client, file, forceDownload, localFilesOnly)
}

func (client *HFClient) SnapshotDownload(repo *HfRepo, forceDownload bool, localFilesOnly bool) (string, error) {
	if repo.Type != ModelRepoType {
		return "", fmt.Errorf("invalid repo type: %s", repo.Type)
	}

	return snapshotDownload(client, repo, forceDownload, localFilesOnly)
}
