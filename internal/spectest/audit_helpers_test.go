package spectest

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func readCommitForTest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "commit" {
			return strings.TrimSpace(v), nil
		}
	}
	return "", fmt.Errorf("no commit: line in %s", path)
}
