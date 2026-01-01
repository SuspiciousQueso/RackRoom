package agent

import "runtime"

func collectInventoryJSON() ([]byte, error) {
	if runtime.GOOS == "windows" {
		return collectWindowsInventoryJSON()
	}
	return nil, nil // later: linux inventory
}
