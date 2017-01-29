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
	doVersionCheck := flag.Bool("do-version-check", true, "Enables version checking.")
	flag.Parse()

	if *doVersionCheck {
		if err := getVersion(*version, *filename); err != nil {
			log.Fatal(err)
		}
	}

	if err := startServer(*filename, flag.Args()); err != nil {
		log.Fatal(err)
	}
}

// startServer starts the server with the given filename and arguments.
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

	// Start the server.
	if err := cmd.Start(); err != nil {
		return err
	}

	// Copy stdin to server input.
	go func() {
		if _, err := io.Copy(in, os.Stdin); err != nil {
			log.Fatal(err)
		}
	}()

	// Copy server output to stdout.
	go func() {
		if _, err := io.Copy(os.Stdout, out); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait for server to exit.
	if err := cmd.Wait(); err != nil {
		return err
	}

	return nil
}

// getVersion obtains the server version with the given id and filename.
func getVersion(id string, filename string) error {
	// versionManifest contains the parsed JSON from the version manifest.
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

	// versionJSON contains the parsed JSON from the version information.
	type versionJSON struct {
		Downloads struct {
			Server struct {
				SHA1 string
				URL  string
			}
		}
	}

	// Get the version manifest.
	var manifest versionManifest
	if err := getJSON("https://launchermeta.mojang.com/mc/game/version_manifest.json", &manifest); err != nil {
		return err
	}

	// Map 'release' and 'snapshot' keywords to the latest versions.
	if id == "release" {
		id = manifest.Latest.Release
	} else if id == "snapshot" {
		id = manifest.Latest.Snapshot
	}

	// Test if the given version is listed in the manifest.
	for _, v := range manifest.Versions {
		if id == v.ID {
			// Obtain the information for the given version.
			var json versionJSON
			if err := getJSON(v.URL, &json); err != nil {
				return err
			}

			// Get the server from the given filename.
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				// Download the file if it doesn't exist.
				if err := downloadFile(filename, json.Downloads.Server.URL); err != nil {
					return err
				}

				if err := verifySHA1(filename, json.Downloads.Server.SHA1); err != nil {
					return err
				}
			} else {
				// Open the file if it exists.
				file, err := os.Open(filename)
				if err != nil {
					return err
				}
				defer file.Close()

				// Attempt to download the file if SHA1 doesn't validate.
				if err := verifySHA1(filename, json.Downloads.Server.SHA1); err != nil {
					if err := downloadFile(filename, json.Downloads.Server.URL); err != nil {
						return err
					}

					// Verify the SHA1 of the newly downloaded file.
					if err := verifySHA1(filename, json.Downloads.Server.SHA1); err != nil {
						return err
					}
				}
			}

			return nil
		}
	}

	return errors.New("invalid version")
}

// getJSON parses JSON from a given url into the given target interface.
func getJSON(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(target)
}

// verifySHA1 verifies a file's SHA1 against the given checksum.
func verifySHA1(filename, checksum string) error {
	// Try to open the file with the given filename.
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Generate a SHA1 hash for the file.
	hash := sha1.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}

	// Test if the hash matches the checksum.
	sha1 := hex.EncodeToString(hash.Sum(nil))
	if sha1 != checksum {
		return errors.New("sha1 checksum doesn't validate")
	}

	return nil
}

// downloadFile downloads a file from the given url into the current directory.
func downloadFile(filename, url string) error {
	// Try to create the file with the given filename.
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get the response from the given url.
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Copy the response body into the file.
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
