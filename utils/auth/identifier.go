package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/pbkdf2"
	"hash"
	"math/big"
	"strconv"
	"strings"
)

func passwordHash(bytePassword, salt []byte, iterations, KeyLen int, hash func() hash.Hash) string {
	return base64.StdEncoding.EncodeToString(pbkdf2.Key(bytePassword, salt, iterations, KeyLen, hash))
}

func saltGenerator(stringLen int) ([]byte, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	charsLength := len(chars)
	var builder bytes.Buffer
	for i := 0; i < stringLen; i++ {
		choiceIndex, err := rand.Int(rand.Reader, big.NewInt(int64(charsLength)))
		if err != nil {
			return nil, err
		}
		err = builder.WriteByte(chars[choiceIndex.Int64()])
		if err != nil {
			return nil, err
		}
	}
	return builder.Bytes(), nil
}

func MakePassword(rawPassword string) (string, error) {
	salt, err := saltGenerator(12)
	if err != nil {
		return "", err
	}
	algorithm := "sha256"
	iterations := 216000
	hashBase64 := passwordHash([]byte(rawPassword), salt, iterations, 32, sha256.New)

	return fmt.Sprintf("pbkdf2_%v$%v$%v$%v", algorithm, iterations, string(salt), hashBase64), nil
}

func CheckPassword(rawPassword, encryptPassword string) (bool, error) {
	splitEncryptedPassword := strings.Split(encryptPassword, "$")
	if len(splitEncryptedPassword) != 4 {
		return false, fmt.Errorf("parse encryptPassword error: %v", encryptPassword)
	}
	algorithm := splitEncryptedPassword[0]
	splitAlgorithm := strings.Split(algorithm, "_")
	if len(splitAlgorithm) != 2 {
		return false, fmt.Errorf("parse encryptPassword algorithm error: %v", encryptPassword)
	}

	var hashOutputSize int
	var hashFactory func() hash.Hash
	if splitAlgorithm[1] == "sha256" {
		hashOutputSize = 32
		hashFactory = sha256.New
	} else {
		return false, fmt.Errorf("invalid sum algorithm: %v", splitAlgorithm[1])
	}

	iterations, err := strconv.Atoi(splitEncryptedPassword[1])
	if err != nil {
		return false, err
	}

	salt := splitEncryptedPassword[2]

	hashBase64 := passwordHash([]byte(rawPassword), []byte(salt), iterations, hashOutputSize, hashFactory)

	return hashBase64 == splitEncryptedPassword[3], nil
}
