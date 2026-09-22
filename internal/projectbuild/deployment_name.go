package projectbuild

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
)

const deploymentNameAttempts = 100

var deploymentAdjectives = []string{
	"admiring", "adventurous", "affectionate", "amazing", "ambitious",
	"brave", "bright", "calm", "charming", "clever",
	"confident", "cool", "determined", "eager", "elegant",
	"epic", "festive", "focused", "friendly", "gifted",
	"gracious", "happy", "hopeful", "inspiring", "jovial",
	"kind", "lucid", "magical", "modest", "nifty",
	"optimistic", "peaceful", "practical", "quirky", "relaxed",
	"resilient", "serene", "sharp", "splendid", "stoic",
	"trusting", "upbeat", "vibrant", "vigilant", "wise",
}

var deploymentPioneers = []string{
	"adams", "allen", "babbage", "banach", "bell",
	"blackwell", "bohr", "booth", "bose", "brahmagupta",
	"cerf", "cohen", "curie", "dijkstra", "dirac",
	"easley", "einstein", "engelbart", "euclid", "faraday",
	"feistel", "fermat", "feynman", "franklin", "galileo",
	"goldberg", "hamilton", "hawking", "hellman", "hopper",
	"hypatia", "johnson", "keller", "kepler", "knuth",
	"lamarr", "leavitt", "lovelace", "mayer", "mccarthy",
	"mcnulty", "mendel", "mirzakhani", "morse", "nash",
	"newton", "noether", "noyce", "pascal", "pasteur",
	"perlman", "pike", "ritchie", "sammet", "shannon",
	"stonebraker", "swartz", "tesla", "thompson", "torvalds",
	"turing", "volta", "wilkes", "williams", "wozniak",
}

func randomDeploymentName() (string, error) {
	firstAdjective, err := randomDeploymentWord(deploymentAdjectives)
	if err != nil {
		return "", err
	}
	secondAdjective, err := randomDeploymentWord(deploymentAdjectives)
	if err != nil {
		return "", err
	}
	pioneer, err := randomDeploymentWord(deploymentPioneers)
	if err != nil {
		return "", err
	}
	return firstAdjective + "-" + secondAdjective + "-" + pioneer, nil
}

func randomDeploymentWord(words []string) (string, error) {
	return words[rand.IntN(len(words))], nil
}

func availableDeploymentArchivePath(root string, generate func() (string, error)) (string, error) {
	directory := filepath.Join(root, ".shopware-cli", "deployments")
	for range deploymentNameAttempts {
		name, err := generate()
		if err != nil {
			return "", err
		}
		output := filepath.Join(directory, name+".tar.gz")
		if _, err := os.Lstat(output); errors.Is(err, os.ErrNotExist) {
			return output, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("could not generate a unique deployment name after %d attempts", deploymentNameAttempts)
}
