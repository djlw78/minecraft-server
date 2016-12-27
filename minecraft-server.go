package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
)

func main() {
	filename := flag.String("filename", "server.jar", "Filename to use for the server.")
	version := flag.String("version", "release", "Minecraft version to use. Must be 'release' (default), 'snapshot', or a specific version string.")
	flag.Parse()

	if err := getVersion(*version, *filename); err != nil {
		log.Fatal(err)
	}

	if err := startServer(*filename, flag.Args()); err != nil {
		log.Fatal(err)
	}
}

func startServer(filename string, args []string) error {
	name := "java"
	args = append(args, "-server", "-jar", filename, "nogui")
	cmd := exec.Command(name, args...)

	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		if _, err := io.Copy(in, os.Stdin); err != nil {
			log.Fatal(err)
		}
	}()

	go func() {
		if _, err := io.Copy(os.Stdout, out); err != nil {
			log.Fatal(err)
		}
	}()

	if err := cmd.Wait(); err != nil {
		return err
	}

	return nil
}

func getVersion(id string, filename string) error {
	type versionManifest struct {
		Latest struct {
			Release  string
			Snapshot string
		}
		Versions []struct {
			ID  string
			URL string
		}
	}

	type versionJSON struct {
		Downloads struct {
			Server struct {
				SHA1 string
				URL  string
			}
		}
	}

	var manifest versionManifest
	if err := getJSON("https://launchermeta.mojang.com/mc/game/version_manifest.json", &manifest); err != nil {
		return err
	}

	if id == "release" {
		id = manifest.Latest.Release
	} else if id == "snapshot" {
		id = manifest.Latest.Snapshot
	}

	for _, v := range manifest.Versions {
		if id == v.ID {
			var json versionJSON
			if err := getJSON(v.URL, &json); err != nil {
				return err
			}

			file, err := os.Create(filename)
			if err != nil {
				return err
			}
			defer file.Close()

			resp, err := http.Get(json.Downloads.Server.URL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			_, err = io.Copy(file, resp.Body)
			if err != nil {
				return err
			}

			hash := sha1.New()
			if _, err = io.Copy(hash, file); err != nil {
				return errors.New("sha1 checksum doesn't validate")
			}

			sha1 := hex.EncodeToString(hash.Sum(nil))
			if sha1 != json.Downloads.Server.SHA1 {
				return err
			}

			return nil
		}
	}

	return errors.New("invalid version")
}

func getJSON(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(target)
}
