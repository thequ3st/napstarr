package p2p

import "os"

// fileHandle wraps os.File for stream/transfer use.
type fileHandle = os.File

func openFile(path string) (*os.File, error) {
	return os.Open(path)
}
