package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

func snapshotDownload(client *HFClient, repo *HfRepo, forceDownload bool, localFilesOnly bool) (string, error) {
	wg := sync.WaitGroup{}
	modelInfo, err := getModelInfo(repo)
	if err != nil && !isOfflineError(err) {
		return "", err
	}

	storageFolder := filepath.Join(client.CacheDir, repoFolderName(repo.Id, repo.Type))

	var commitHash string
	if modelInfo == nil {
		if regexp.MustCompile(CommitHashPattern).MatchString(repo.Revision) {
			commitHash = repo.Revision
		} else {
			refPath := filepath.Join(storageFolder, "refs", repo.Revision)
			_, err := os.Stat(refPath)
			if err == nil {
				content, err := os.ReadFile(refPath)
				if err != nil {
					return "", err
				}
				commitHash = string(content)
			} else {
				return "", err
			}
		}

		if commitHash != "" {
			snapshotFolder := filepath.Join(storageFolder, "snapshots", commitHash)
			_, err := os.Stat(snapshotFolder)
			if err == nil {
				return snapshotFolder, nil
			}
			if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
	}

	if localFilesOnly {
		return "", fmt.Errorf(
			"cannot find an appropriate cached snapshot folder for the specified revision on the local disk and outgoing traffic has been disabled. To enable repo look-ups and downloads online, set localFilesOnly to false",
		)
	}

	if modelInfo.Sha == "" {
		return "", fmt.Errorf("no sha found for this model")
	}

	if modelInfo.Siblings == nil {
		return "", fmt.Errorf("no siblings found for this model")
	}

	commitHash = modelInfo.Sha
	snapshotFolder := filepath.Join(storageFolder, "snapshots", commitHash)

	if repo.Revision != commitHash {
		refPath := filepath.Join(storageFolder, "refs", repo.Revision)
		_, err = os.Stat(refPath)
		if err == nil {
			err := os.WriteFile(refPath, []byte(commitHash), os.ModePerm)
			if err != nil {
				return "", err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}

	download := func(fileName string) {
		wg.Add(1)
		defer wg.Done()
		fileDownload(client, repo.File(fileName), forceDownload, localFilesOnly)
	}

	for _, sibling := range modelInfo.Siblings {
		go download(sibling.RFileName)
	}

	wg.Wait()

	return snapshotFolder, nil
}

func getModelInfo(repo *HfRepo) (*HFModelInfo, error) {
	headers := map[string]string{
		"User-Agent": DefaultUserAgent,
	}

	if repo.Type != ModelRepoType {
		return nil, fmt.Errorf("invalid repo type: %s", repo.Type)
	}

	var url string
	if repo.Revision != "" {
		url = fmt.Sprintf("https://huggingface.co/api/models/%s/revision/%s", repo.Id, repo.Revision)
	} else {
		url = fmt.Sprintf("https://huggingface.co/api/models/%s", repo.Id)
	}

	response, err := requestWrapper("GET", url, false, headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	var data = &HFModelInfo{}
	err = json.NewDecoder(response.Body).Decode(&data)
	if err != nil {
		return nil, err
	}

	return data, nil
}
