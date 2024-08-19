Interfaces from Hugging Face Model Hub that we're using currently:

- function `hf_hub_download`; downloads a single specified file:

```py
def hf_hub_download(
    repo_id: str,
    filename: str,
    *,
    subfolder: Optional[str] = None,
    repo_type: Optional[str] = None,
    revision: Optional[str] = None,
    library_name: Optional[str] = None,
    library_version: Optional[str] = None,
    cache_dir: Union[str, Path, None] = None,
    local_dir: Union[str, Path, None] = None,
    user_agent: Union[Dict, str, None] = None,
    force_download: bool = False,
    proxies: Optional[Dict] = None,
    etag_timeout: float = DEFAULT_ETAG_TIMEOUT,
    token: Union[bool, str, None] = None,
    local_files_only: bool = False,
    headers: Optional[Dict[str, str]] = None,
    endpoint: Optional[str] = None,
    # Deprecated args
    legacy_cache_layout: bool = False,
    resume_download: Optional[bool] = None,
    force_filename: Optional[str] = None,
    local_dir_use_symlinks: Union[bool, Literal["auto"]] = "auto",
) -> str:
```

However these arguments are sufficient:

```py
repo_id,
file_name,
subfolder,
cache_dir,
```

- `snapshot_download`; used to download the entire repo (all files), but we're using an include pattern to only download a single sub-folder, rather than the entire repo:

```py
def snapshot_download(
    repo_id: str,
    *,
    repo_type: Optional[str] = None,
    revision: Optional[str] = None,
    cache_dir: Union[str, Path, None] = None,
    local_dir: Union[str, Path, None] = None,
    library_name: Optional[str] = None,
    library_version: Optional[str] = None,
    user_agent: Optional[Union[Dict, str]] = None,
    proxies: Optional[Dict] = None,
    etag_timeout: float = DEFAULT_ETAG_TIMEOUT,
    force_download: bool = False,
    token: Optional[Union[bool, str]] = None,
    local_files_only: bool = False,
    allow_patterns: Optional[Union[List[str], str]] = None,
    ignore_patterns: Optional[Union[List[str], str]] = None,
    max_workers: int = 8,
    tqdm_class: Optional[base_tqdm] = None,
    headers: Optional[Dict[str, str]] = None,
    endpoint: Optional[str] = None,
    # Deprecated args
    local_dir_use_symlinks: Union[bool, Literal["auto"]] = "auto",
    resume_download: Optional[bool] = None,
) -> str:
```

'Allow pattern' and 'ignore pattern' are the useful parts here.

- `scan_cache_dir`; used to get info about what repos we already have available:

```py
def scan_cache_dir(cache_dir: Optional[Union[str, Path]] = None) -> HFCacheInfo:
	pass

class HFCacheInfo:
    size_on_disk: int
    repos: FrozenSet[CachedRepoInfo]
    warnings: List[CorruptedCacheException]

	def delete_revisions(self, *revisions: str) -> DeleteCacheStrategy:
		pass

@dataclass(frozen=True)
class CachedRepoInfo:
    repo_id: str
    repo_type: REPO_TYPE_T
    repo_path: Path
    size_on_disk: int
    nb_files: int
    revisions: FrozenSet[CachedRevisionInfo]
    last_accessed: float
    last_modified: float
```

However, all we're doing with this CachedRepoInfo is looking through every repo-id and then seeing if they're downloaded using custom-logic that should have been written into the library by default.

- repo_folder_name function (trivial):

```py
def repo_folder_name(*, repo_id: str, repo_type: str) -> str:
    """Return a serialized version of a hf.co repo name and type, safe for disk storage
    as a single non-nested folder.

    Example: models--julien-c--EsperBERTo-small
    """
    # remove all `/` occurrences to correctly convert repo to directory name
    parts = [f"{repo_type}s", *repo_id.split("/")]
    return REPO_ID_SEPARATOR.join(parts)
```

- class `HFApi`:

```py
class HfApi:
    def __init__(
        self,
        endpoint: Optional[str] = None,
        token: Union[str, bool, None] = None,
        library_name: Optional[str] = None,
        library_version: Optional[str] = None,
        user_agent: Union[Dict, str, None] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> None:
		pass

    def list_models(
        self,
        *,
        filter: Union[ModelFilter, str, Iterable[str], None] = None,
        author: Optional[str] = None,
        library: Optional[Union[str, List[str]]] = None,
        language: Optional[Union[str, List[str]]] = None,
        model_name: Optional[str] = None,
        task: Optional[Union[str, List[str]]] = None,
        trained_dataset: Optional[Union[str, List[str]]] = None,
        tags: Optional[Union[str, List[str]]] = None,
        search: Optional[str] = None,
        emissions_thresholds: Optional[Tuple[float, float]] = None,
        sort: Union[Literal["last_modified"], str, None] = None,
        direction: Optional[Literal[-1]] = None,
        limit: Optional[int] = None,
        full: Optional[bool] = None,
        cardData: bool = False,
        fetch_config: bool = False,
        token: Union[bool, str, None] = None,
        pipeline_tag: Optional[str] = None,
    ) -> Iterable[ModelInfo]:
```

It has many methods, but we're only using its `list_models` method so far, which actually does nothing important right now (the client can query for available diffusers models) so honestly we don't even need this at all right now.

### How We Want to Design this Library:

There are a few things we want to add which were missing in the original huggingface-hub python package. Honestly, with the exception of `hf_hub_download`, which is a general-purpose single-file downloader, we don't need any of these classes or methods at all.

Honestly, all we want to do is be able to (1) see what models are available locally already, and (2) download diffusers models from repos, but only the files that we actually need. We can create some function like:

```py
def download_diffusers_pipeline(
	repo_id: str,
	*,
	revision: str,
	model_index: str,
	components: dict[str, dict[str, str]]
) -> None:
	pass
```
The arguments, in a config.yml file, would look something like this:

```yml
repo_id: CompVis/stable-diffusion-v1-4
revision: main
model_index: path/to/model-index.json # overwrites base_repo's model-index file if specified

components:
	# unspecified components will use the base_repo as a source
	# otherwise:
  text_encoder:
	# because a folder is specified, rather than a single file, we download just
	# the files we need, which in this case is just config.json and model.fp16.safetensors
    hf_repo: stabilityai/stable-diffusion-xl-base-1.0/text_encoder
	variant: fp16
	revision: main
  vae:
	# we specify a list of individual files contained in the same repo / folder
    source: [
		stabilityai/stable-diffusion-xl-base-1.0/vae/diffusion_pytorch_model.fp16.safetensors,
		stabilityai/stable-diffusion-xl-base-1.0/vae/config.json
	]
    revision: main
  unet:
    local_path: /path/to/local/unet
  transformer:
    urls: {
		1234.safetensors: https://civitai.com/models/1234.safetensors, 
		config.json: https://civitai.com/models/1234.json
	}
```

Given how complex this components type is, we may want to create a class (struct) in order to represent all of this

- `is_available_locally(repo-id): bool` function; some way of telling if we have sufficient files available locally. This would ensure that we have all the files necessary:

```py
def is_available_locally(
	repo_id: str,
	*,
	revision: str,
	components: dict[str, dict[str, str]]
) -> None:
	pass
```

Interestingly, in workflow files, for these diffusers-pipeline definitions, we could expliclity provide the above information, or we could link to it in a yaml file. For example, ex: `cozy-creator/diffusers-pipe-defs/flux-fp8.yaml` would be a link to a diffusers-pipeline file on hugigng face in our own organization.

I like using the term `diffusers pipeline` because it's more specific than just 'model'.

- Note that we also want to provide progress-bars to clients as files are downloaded, so we can report what files are being downloaded and how long it'll be until they're complete.

### References:

* [huggingface_hub](https://github.com/huggingface/huggingface_hub)
* [hf-hub](https://github.com/huggingface/hf-hub)

Golang adaptation was originally forked from [seasonjs/hf-hub](https://github.com/seasonjs/hf-hub)

