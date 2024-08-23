// Copyright (c) seasonjs. All rights reserved.
// Licensed under the MIT License. See License.txt in the project root for license information.

package main

import (
	"fmt"
	"log"

	"github.com/cozy-creator/hf-hub/hub"
)

func main() {
	client := hub.DefaultClient().WithCacheDir("./models")
	repo := hub.NewHfRepo("black-forest-labs/FLUX.1-schnell")
	file := repo.File("schnell_grid.jpeg")

	path, err := client.FileDownload(file, true, false)
	// path, err := client.SnapshotDownload(repo, false, false)
	if err != nil {
		log.Println(err)
	}

	fmt.Println(path)
}
