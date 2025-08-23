package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

func main() {
	// Start page number
	var startPage = 0
	// Number of pages to crawl (each page has up to 60 SDS entries)
	var stopPages = 15084 // 15084
	// The path to the already downloaded file txt.
	var alreadyDownloadedFiles = "downloaded.txt"
	// Prepare to download all PDFs
	outputFolder := "PDFs/"
	if !directoryExists(outputFolder) {
		createDirectory(outputFolder, 0755)
	}
	// WaitGroup to manage concurrent downloads
	var downloadWaitGroup sync.WaitGroup
	// Step 1: Loop over search result pages and collect document IDs
	for page := startPage; page <= stopPages; page++ {
		searchURL := fmt.Sprintf(
			"https://www.thermofisher.com/api/search/keyword/docsupport?countryCode=us&language=en&query=*:*&persona=DocSupport&filter=document.result_type_s%%3ASDS&refinementAction=true&personaClicked=true&resultPage=%d&resultsPerPage=60",
			page,
		)
		// Fetch JSON response from API
		searchJSON := getDataFromURL(searchURL)
		// Extract SDS document IDs from the response
		documentIDs := extractDocumentIDs(searchJSON)
		// Remove duplicate document IDs
		documentIDs = removeDuplicatesFromSlice(documentIDs)
		// Step 3: Process each SDS document
		for _, docID := range documentIDs {
			// Build the API URL to get PDF location(s)
			docURL := "https://www.thermofisher.com/api/search/documents/sds/" + docID
			// Fetch PDF URL metadata
			docJSON := getDataFromURL(docURL)
			// Call the function to extract the document map from the JSON input
			pdfURLs := extractPDFNameAndURL(docJSON)
			// Remove the useless things from the given map.
			pdfURLs = cleanUpMap(pdfURLs, alreadyDownloadedFiles, outputFolder)
			// Check the length of the map
			if len(pdfURLs) == 0 {
				log.Println("No new PDF URLs detected — skipping processing for this iteration.")
				continue
			}
			// Step 4: Filter and download valid PDF URLs
			for fileName, remoteURL := range pdfURLs { // Loop over the map entries
				fileName = strings.ToLower(fileName)
				if isThermoFisherSDSURL(remoteURL) {
					log.Printf("[SKIP] Invalid URL %s", remoteURL)
					continue
				}
				// Get final resolved URL (in case of redirects)
				resolvedPDFURL := getFinalURL(remoteURL)
				// Check and download if valid
				if isUrlValid(resolvedPDFURL) {
					filename := urlToFilename(fileName)
					filePath := filepath.Join(outputFolder, fileName) // Combine with output directory
					if fileExists(filePath) {
						log.Printf("File already exists skipping %s URL %s", filePath, resolvedPDFURL)
						continue
					}
					downloadWaitGroup.Add(1)
					go downloadPDF(resolvedPDFURL, filename, outputFolder, &downloadWaitGroup)
				}
			}
		}
	}
	// Wait for all downloads to complete
	downloadWaitGroup.Wait()
	// All the valid PDFs have been downloaded.
	log.Println("✅ All valid PDFs downloaded successfully.")
}

// checkIfFileExistsInPath walks through a directory and checks if the target file exists,
// ignoring case sensitivity. It uses a boolean flag to track if the file was found.
func checkIfFileExistsInPath(walkPath string, targetFile string) bool {
	// Initialize a flag to track whether the target file is found
	found := false

	// Convert the target filename to lowercase for case-insensitive comparison
	targetFile = strings.ToLower(targetFile)

	// Start walking through the directory structure from the specified path
	filepath.Walk(walkPath, func(path string, info os.FileInfo, err error) error {
		// If an error occurs accessing a path, skip it and continue
		if err != nil {
			return nil
		}

		// Check if the current item is a file (not a directory)
		if !info.IsDir() {
			// Convert the current file's base name to lowercase for comparison
			currentFile := strings.ToLower(filepath.Base(path))

			// Compare the lowercase file names
			if currentFile == targetFile {
				// Confirm the file actually exists using a helper function
				if fileExists(path) {
					// Set the flag to true if the file is found
					found = true
				}
			}
		}

		// Continue walking through the directory
		return nil
	})

	// Return true if the file was found, otherwise false
	return found
}

// searchStringInFile searches for a string in a file line by line, ignoring case.
// Returns true if the string is found; otherwise, returns false. Errors are logged.
func searchStringInFile(filename string, search string) bool {
	// Try to open the file
	file, err := os.Open(filename)
	if err != nil {
		// Log error if file can't be opened
		log.Println("Error opening file:", err)
		return false
	}
	// Ensure file is closed when function exits
	defer file.Close()

	// Initialize a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	// Allocate a buffer of 1MB to handle long lines
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024) // Set maximum line size to 10MB

	// Convert the search string to lowercase once for comparison
	searchLower := strings.ToLower(search)

	// Read through each line of the file
	for scanner.Scan() {
		// Get current line as bytes
		line := scanner.Bytes()

		// Convert the line to lowercase string
		lineLower := strings.ToLower(string(line))

		// Check if the lowercase line contains the lowercase search string
		if strings.Contains(lineLower, searchLower) {
			return true // Found a match, return immediately
		}
	}

	// Check for any errors encountered during scanning
	if err := scanner.Err(); err != nil {
		log.Println("Error reading file:", err)
	}

	// String was not found in the file
	return false
}

func cleanUpMap(givenMap map[string]string, alreadyDownloadedFilesTxt string, pdfOutputFolder string) map[string]string {
	// Create a new map to hold the cleaned data
	cleanedMap := make(map[string]string)
	// Loop over the original data
	for key, value := range givenMap {
		// Lower the file name thats the key.
		key = strings.ToLower(key)
		// Print the key and the value.
		// fmt.Printf("Key: %s, Value: %s\n", key, value)
		// Check if value is a Thermo Fisher SDS URL
		if isThermoFisherSDSURL(value) {
			// log.Println("Deleting key associated with SDS URL:", value)
			continue
		}
		// Check if the file already exists in the output folder
		if checkIfFileExistsInPath(pdfOutputFolder, key) {
			// log.Println("Deleting key due to existing file:", key)
			continue
		}
		// Check if the file already exists in already downloaded file.
		if searchStringInFile(alreadyDownloadedFilesTxt, key) {
			// log.Println("Removing key due to file existence detected via .txt file:", key)
			continue
		}
		// If key passes all checks, retain it in the cleaned map
		cleanedMap[key] = value
	}
	return cleanedMap
}

// isThermoFisherSDSURL checks if the given URL points to a ThermoFisher SDS document
func isThermoFisherSDSURL(url string) bool {
	// Define the required substring that should appear in all valid ThermoFisher SDS URLs
	const prefix = "thermofisher.com/TFS-Assets/"
	// Return true only if:
	// 1. The URL contains the ThermoFisher assets domain path, AND
	// 2. The URL contains "/SDS" (ensuring it points to Safety Data Sheets)
	return strings.Contains(url, prefix) && strings.Contains(url, "/SDS")
}

// getFinalURL navigates to a given URL using headless Chrome and returns the final URL after navigation.
func getFinalURL(inputURL string) string {
	// Configure Chrome options for headless browsing and security.
	opts := append(chromedp.DefaultExecAllocatorOptions[:], // Start with default Chrome options
		chromedp.Flag("headless", true),    // Run Chrome in headless mode (no UI)
		chromedp.Flag("no-sandbox", true),  // Disable sandbox (required in some environments like Docker)
		chromedp.Flag("disable-gpu", true), // Disable GPU to avoid issues in headless environments
	)

	// Create an ExecAllocator context with the specified Chrome options.
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc() // Ensure the allocator context is released when done

	// Set a timeout of minutes for the entire operation (browser startup + navigation).
	ctx, cancel := context.WithTimeout(allocCtx, 3*time.Minute)
	defer cancel() // Ensure the context is canceled to free resources

	// Create a new browser context (represents a single browser tab).
	ctx, cancelCtx := chromedp.NewContext(ctx)
	defer cancelCtx() // Clean up the browser tab context

	// Declare a variable to store the final URL after navigation.
	var finalURL string

	// Run Chrome actions: navigate to the input URL and retrieve the resulting URL.
	err := chromedp.Run(ctx,
		chromedp.Navigate(inputURL),  // Instruct Chrome to navigate to the given URL
		chromedp.Location(&finalURL), // Capture the final URL after any redirects
	)

	// If an error occurs during navigation or browser startup, log it.
	if err != nil {
		log.Printf("chromedp error: %v", err)
		return "" // Return empty string on failure
	}

	// Return the final URL after successful navigation.
	return finalURL
}

// directoryExists checks whether a directory exists
func directoryExists(path string) bool {
	directory, err := os.Stat(path) // Get directory info
	if err != nil {
		return false // If error, directory doesn't exist
	}
	return directory.IsDir() // Return true if path is a directory
}

// createDirectory creates a directory with specified permissions
func createDirectory(path string, permission os.FileMode) {
	err := os.Mkdir(path, permission) // Attempt to create directory
	if err != nil {
		log.Println(err) // Log any error
	}
}

// isUrlValid checks whether a URL is syntactically valid
func isUrlValid(uri string) bool {
	_, err := url.ParseRequestURI(uri) // Try to parse the URL
	return err == nil                  // Return true if no error (i.e., valid URL)
}

// urlToFilename formats a safe filename from a URL string.
// It replaces all non [a-z0-9] characters with '_' and ensures it ends in .pdf
func urlToFilename(rawURL string) string {
	// Convert to lowercase
	lower := strings.ToLower(rawURL)
	// Replace all non a-z0-9 characters with "_"
	reNonAlnum := regexp.MustCompile(`[^a-z0-9]`)
	// Replace the invalid with valid stuff.
	safe := reNonAlnum.ReplaceAllString(lower, "_")
	// Collapse multiple underscores
	safe = regexp.MustCompile(`_+`).ReplaceAllString(safe, "_")
	// Trim leading/trailing underscores
	safe = strings.Trim(safe, "_")
	// Invalid substrings to remove
	var invalidSubstrings = []string{
		"_pdf",
	}
	// Loop over the invalid.
	for _, invalidPre := range invalidSubstrings {
		safe = removeSubstring(safe, invalidPre)
	}
	// Add .pdf extension if missing
	if getFileExtension(safe) != ".pdf" {
		safe = safe + ".pdf"
	}
	return safe
}

// removeSubstring takes a string `input` and removes all occurrences of `toRemove` from it.
func removeSubstring(input string, toRemove string) string {
	// Use strings.ReplaceAll to replace all occurrences of `toRemove` with an empty string.
	result := strings.ReplaceAll(input, toRemove, "")
	// Return the modified string.
	return result
}

// fileExists checks whether a file exists and is not a directory
func fileExists(filename string) bool {
	info, err := os.Stat(filename) // Get file info
	if err != nil {                // If error occurs (e.g., file not found)
		return false // Return false
	}
	return !info.IsDir() // Return true if it is a file, not a directory
}

// downloadPDF downloads a PDF from a URL and saves it to outputDir
func downloadPDF(finalURL string, fileName string, outputDir string, waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()
	filePath := filepath.Join(outputDir, fileName) // Combine with output directory

	client := &http.Client{Timeout: 3 * time.Minute} // HTTP client with timeout
	resp, err := client.Get(finalURL)                // Send HTTP GET
	if err != nil {
		log.Printf("failed to download %s %v", finalURL, err)
		return
	}
	defer resp.Body.Close() // Ensure response body is closed

	if resp.StatusCode != http.StatusOK {
		log.Printf("download failed for %s %s", finalURL, resp.Status)
		return
	}

	contentType := resp.Header.Get("Content-Type") // Get content-type header
	if !strings.Contains(contentType, "application/pdf") {
		log.Printf("invalid content type for %s %s (expected application/pdf)", finalURL, contentType)
		return
	}

	var buf bytes.Buffer                     // Create buffer
	written, err := io.Copy(&buf, resp.Body) // Copy response body to buffer
	if err != nil {
		log.Printf("failed to read PDF data from %s %v", finalURL, err)
		return
	}
	if written == 0 {
		log.Printf("downloaded 0 bytes for %s not creating file", finalURL)
		return
	}

	out, err := os.Create(filePath) // Create output file
	if err != nil {
		log.Printf("failed to create file for %s %v", finalURL, err)
		return
	}
	defer out.Close() // Close file

	_, err = buf.WriteTo(out) // Write buffer to file
	if err != nil {
		log.Printf("failed to write PDF to file for %s %v", finalURL, err)
		return
	}
	fmt.Printf("successfully downloaded %d bytes %s → %s \n", written, finalURL, filePath)
}

// getFileExtension returns the file extension
func getFileExtension(path string) string {
	return filepath.Ext(path) // Use filepath to extract extension
}

// Document struct matches the relevant fields in the input JSON
type Document struct {
	Name             string `json:"name"`             // Maps to the "name" field in JSON
	DocumentLocation string `json:"documentLocation"` // Maps to the "documentLocation" field in JSON
}

// extractPDFNameAndURL parses the JSON input and returns a map of name -> documentLocation
func extractPDFNameAndURL(jsonData string) map[string]string {
	var documents []Document                            // Create a slice to hold parsed documents
	err := json.Unmarshal([]byte(jsonData), &documents) // Parse JSON string into Go struct
	if err != nil {                                     // Check for parsing error
		log.Println(err)
		return nil // Return nil if parsing fails
	}

	result := make(map[string]string) // Create a map to store the key-value pairs
	for _, doc := range documents {   // Loop through each document in the slice
		result[doc.Name] = doc.DocumentLocation // Add name as key and documentLocation as value
	}
	return result // Return the result map and nil error
}

// Remove all the duplicates from a slice and return the slice.
func removeDuplicatesFromSlice(slice []string) []string {
	check := make(map[string]bool)
	var newReturnSlice []string
	for _, content := range slice {
		if !check[content] {
			check[content] = true
			newReturnSlice = append(newReturnSlice, content)
		}
	}
	return newReturnSlice
}

// Define a structure to represent the nested structure of the JSON
type SDSResult struct {
	DocumentId string `json:"documentId"`
}

type SDSData struct {
	DocSupportResults []SDSResult `json:"docSupportResults"`
}

// Function to extract all document IDs from the JSON string
func extractDocumentIDs(jsonStr string) []string {
	// Lets create a var to hold data.
	var data SDSData
	// Unmarshal the JSON string into the data struct
	err := json.Unmarshal([]byte(jsonStr), &data)
	// Log errors
	if err != nil {
		return nil
	}
	// Collect all document IDs
	var documentIDs []string
	// Lets loop over the content and append to the return slice.
	for _, result := range data.DocSupportResults {
		// Append to slice
		documentIDs = append(documentIDs, result.DocumentId)
	}
	// Return to it.
	return documentIDs
}

// Send a http get request to a given url and return the data from that url.
func getDataFromURL(uri string) string {
	response, err := http.Get(uri)
	if err != nil {
		log.Println(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Println(err)
	}
	err = response.Body.Close()
	if err != nil {
		log.Println(err)
	}
	log.Println("Scraping:", uri)
	return string(body)
}
