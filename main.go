package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

const containerFileToken = "==>"

func handleError(err error) {
	fmt.Fprintf(os.Stderr, "ERROR: %s\n", err.Error())
	os.Exit(1)
}

var contentDetector = map[string]string{
	`Starting controllers on`:                    "controllers",
	`msg="start registry" distribution_version=`: "docker-registry",
	`Registered admission plugin`:                "api-server",
	`Starting template router`:                   "router",
	`etcdserver: setting up the initial cluster`: "etcd",
}

type message struct {
	Log string `json:"log"`
}

type logWriter struct {
	fileWriter    io.WriteCloser
	count         int64
	detectedType  string
	containerName string
	dirName       string
}

func (w *logWriter) Write(p []byte) (int, error) {
	var m message
	if len(p) == 0 {
		return 0, nil
	}
	err := json.Unmarshal(p, &m)
	if err != nil {
		return 0, err
	}
	w.count++
	for pattern, fileType := range contentDetector {
		if strings.Contains(m.Log, pattern) {
			w.detectedType = fileType
			break
		}
	}
	return w.fileWriter.Write([]byte(m.Log))
}

func (w *logWriter) Close() error {
	detected := ""
	if len(w.detectedType) > 0 {
		detected = ", detected: " + w.detectedType
		defer func() {
			logFile := path.Join(w.dirName, w.containerName+".log")
			err := os.Rename(logFile, path.Join(w.dirName, w.containerName+"-"+w.detectedType+".log"))
			if err != nil {
				handleError(err)
			}
		}()
	}
	defer w.fileWriter.Close()
	fmt.Fprintf(os.Stdout, "%d lines%s\n", w.count, detected)
	return nil
}

func newWriter(dirName, containerName string) (io.WriteCloser, error) {
	logFile := path.Join(dirName, containerName+".log")
	fmt.Fprintf(os.Stdout, "Writing containers/%s.log ... ", containerName)
	f, err := os.Create(logFile)
	if err != nil {
		return nil, err
	}
	return &logWriter{
		fileWriter:    f,
		containerName: containerName,
		dirName:       dirName,
	}, nil
}

func main() {
	if len(os.Args) != 2 {
		handleError(fmt.Errorf("usage: %s containers.log", os.Args[0]))
	}

	file, err := os.Open(os.Args[1])
	if err != nil {
		handleError(err)
	}
	defer file.Close()

	cwd, err := os.Getwd()
	if err != nil {
		handleError(err)
	}
	containersDir := path.Join(cwd, "containers")
	if err := os.MkdirAll(containersDir, 0700); err != nil {
		handleError(err)
	}

	scan := bufio.NewScanner(file)

	var writer io.WriteCloser
	defer func() {
		if writer != nil {
			writer.Close()
		}
	}()

	for scan.Scan() {
		if err := scan.Err(); err != nil {
			handleError(err)
		}
		line := scan.Bytes()
		if bytes.HasPrefix(line, []byte(containerFileToken)) {
			if writer != nil {
				writer.Close()
			}
			writer, err = newWriter(containersDir, strings.Split(strings.TrimPrefix(string(line), containerFileToken), "/")[5])
			if err != nil {
				handleError(err)
			}
			continue
		}
		if _, err := writer.Write(line); err != nil {
			handleError(err)
		}
	}
}
