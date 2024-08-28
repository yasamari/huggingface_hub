## HF Hub Client

This is a client for the Hugging Face Hub. It allows you to download files from the Hub, and also to download specific revisions of a repo.

It aims to be a drop-in replacement for the original `huggingface_hub` python package, meaning that any model you can download using the python package, you can download using this client and vice versa.

### Requirements

- Go 1.22+

### Installation

```bash
go get github.com/cozy-creator/hf-hub
```

### Usage

#### Initializing the Client

You can initialize the client in two ways, either by using the default client or by creating a new client.

##### Default Client

The default client is initialized by calling the `DefaultClient` function, which returns a client that is configured to use the default Hugging Face Hub endpoint and cache directory.

example:
```go
client := hub.DefaultClient()
```

##### Custom Client

You can also create a custom client by specifying the endpoint, token, and cache directory:

```go
client := hub.NewClient("https://huggingface.co", "your-token", "./models")
```

##### Customizing the Client
The client has several methods that allow you to customize its behavior. These methods are:

The `WithCacheDir` method allows you to change or specify the directory where the client will store the downloaded files.

example:
```go
client := hub.DefaultClient().WithCacheDir("./models")
```

The `WithEndpoint` method allows you to change or specify the endpoint.
```go
client := hub.DefaultClient().WithEndpoint("https://huggingface.co")
```

The `WithToken` method allows you to change or specify the huggingface token.
```go
client := hub.DefaultClient().WithToken("your-token")
```

#### Downloading a repo

The `Download` method allows you to download a model from the Hugging Face Hub. It takes a `DownloadParams` object as an argument, and returns the path to the downloaded repo snapshot.

example:
```go
client := hub.DefaultClient()
repo := hub.NewRepo("black-forest-labs/FLUX.1-schnell")
params := &hub.DownloadParams{Repo: repo}

path, err := client.Download(params)
if err != nil {
	log.Println(err)
  os.Exit(1)
}

fmt.Println(`Repo downloaded to: `, path)
```


#### Downloading a File

You also have the option to download a single file from a repo. This is done by calling the `Download` method on the `DownloadParams` object, but with the `FileName` field set to the name of the file you want to download.

example:
```go
client := hub.DefaultClient()
repo := hub.NewRepo("black-forest-labs/FLUX.1-schnell")
params := &hub.DownloadParams{
  Repo: repo, 
  FileName: "flux1-schnell.safetensors",
}

path, err := client.Download(params)
if err != nil {
	log.Println(err)
  os.Exit(1)
}

fmt.Println(`File downloaded to: `, path)
```

You can also specify a sub-folder to download, by setting the `SubFolder` field of the `DownloadParams` object.

example:
```go
client := hub.DefaultClient()
repo := hub.NewRepo("black-forest-labs/FLUX.1-schnell")
params := &hub.DownloadParams{
  Repo: repo, 
  FileName: "diffusion_pytorch_model.safetensors",
  SubFolder: "vae",
}

path, err := client.Download(params)
if err != nil {
	log.Println(err)
  os.Exit(1)
}

fmt.Println(`File downloaded to: `, path)
```

#### Downloading a Repo Revision

You can also specify a specific revision of a repo to download. This is done by calling the `WithRevision` method on the `Repo` object, and passing the revision you want to download.

example:
```go
client := hub.DefaultClient()
// this revision could also be a branch name, or a commit hash
repo := hub.NewRepo("black-forest-labs/FLUX.1-schnell").WithRevision("main")
params := &hub.DownloadParams{Repo: repo}

path, err := client.Download(params)
if err != nil {
	log.Println(err)
  os.Exit(1)
}

fmt.Println(`Repo downloaded to: `, path)
```

### Contributing

Contributions are welcome! This is still in early development, so there are likely to be some rough edges.
If you find a bug or have a suggestion, please open an issue or submit a pull request.

### Acknowledgements

This project was inspired by the [huggingface_hub](https://github.com/huggingface/huggingface_hub) python package, and the [hf-hub](https://github.com/huggingface/hf-hub) rust crate.

Also, thanks to [seasonjs](https://github.com/seasonjs) for the original [hf-hub](https://github.com/seasonjs/hf-hub) golang package before we did a complete rewrite.

### License

MIT