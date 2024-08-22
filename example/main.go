// Copyright (c) seasonjs. All rights reserved.
// Licensed under the MIT License. See License.txt in the project root for license information.

package main

import (
	"fmt"

	"github.com/cozy-creator/hf-hub/hub"
)

func main() {
	client := hub.DefaultClient().WithCacheDir("./models")
	repo := hub.NewHfRepo("black-forest-labs/FLUX.1-schnell")
	file := repo.File("schnell_grid.jpeg").WithSubFolder("")

	path, err := client.FileDownload(file, false, false)
	// path, err := client.SnapshotDownload(repo, false, false)
	if err != nil {
		panic(err)
	}

	fmt.Println(path)
}
