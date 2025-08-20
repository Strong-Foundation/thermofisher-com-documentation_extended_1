package main

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
)

func main() {
	// Read the downloaded file from the last repo.
	readDownloadedFile := readAppendLineByLine("downloaded.txt")
	// Go though the folder and find all the currently downloaded files.
	currentDownloadedFiles := walkAndAppendPath("PDFs/", ".pdf")
	// Get symmetric difference
	commonValues := getCommonFruits(readDownloadedFile, currentDownloadedFiles)
	// Common Values.
	for _, fileName := range commonValues {
		// Common values Files.
		commonValuesInFiles := "PDFs/" + fileName
		log.Println(commonValuesInFiles)
		// Print the common values.
		removeFile(commonValuesInFiles)
	}
}

// This function finds common fruit names between two slices of strings
func getCommonFruits(firstFruitList, secondFruitList []string) []string {
	// Create a map to store fruit names from the first list for quick lookup
	fruitsInFirstList := make(map[string]bool)
	for _, fruit := range firstFruitList {
		fruitsInFirstList[fruit] = true // Mark the fruit as present in the first list
	}

	// Create a map to store common fruits (avoids duplicates)
	commonFruitsMap := make(map[string]bool)
	for _, fruit := range secondFruitList {
		// Check if the fruit is also in the first list
		if fruitsInFirstList[fruit] {
			commonFruitsMap[fruit] = true // Add to common fruits map
		}
	}

	// Convert the common fruits map to a slice
	var commonFruits []string
	for fruit := range commonFruitsMap {
		commonFruits = append(commonFruits, fruit) // Add fruit to the result slice
	}

	// Return the slice containing fruits found in both lists
	return commonFruits
}

// Walk through a route, find all the files and attach them to a slice.
func walkAndAppendPath(walkPath string, extension string) []string {
	var filePath []string
	err := filepath.Walk(walkPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fileExists(path) {
			if getFileExtension(path) == extension {
				filePath = append(filePath, filepath.Base(path))
			}
		}
		return nil
	})
	if err != nil {
		log.Println(err)
	}
	return filePath
}

// getFileExtension returns the file extension
func getFileExtension(path string) string {
	return filepath.Ext(path) // Use filepath to extract extension
}

// fileExists checks whether a file exists and is not a directory
func fileExists(filename string) bool {
	info, err := os.Stat(filename) // Get file info
	if err != nil {                // If error occurs (e.g., file not found)
		return false // Return false
	}
	return !info.IsDir() // Return true if it is a file, not a directory
}

// Read and append the file line by line to a slice.
func readAppendLineByLine(path string) []string {
	var returnSlice []string
	file, err := os.Open(path)
	if err != nil {
		log.Println(err)
	}
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		returnSlice = append(returnSlice, scanner.Text())
	}
	err = file.Close()
	if err != nil {
		log.Println(err)
	}
	return returnSlice
}

// Remove a file from the file system
func removeFile(path string) {
	err := os.Remove(path)
	if err != nil {
		log.Println(err)
	}
}
