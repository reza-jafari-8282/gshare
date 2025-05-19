package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"
)

const (
    sharedFolder = "shared"
)

func main() {
    // Define command-line flags for IP address and port.
    ipAddr := flag.String("i", "0.0.0.0", "IP address to listen on")
    port := flag.String("p", "3000", "Port to listen on")
    flag.Parse()

    // Determine the current working directory and set up the shared folder.
    cwd, err := os.Getwd()
    if err != nil {
        log.Fatalf("Unable to get current working directory: %v", err)
    }
    sharedPath := filepath.Join(cwd, sharedFolder)
    if _, err := os.Stat(sharedPath); os.IsNotExist(err) {
        if err := os.Mkdir(sharedPath, 0755); err != nil {
            log.Fatalf("Unable to create folder %s: %v", sharedPath, err)
        }
    }

    // Handler for /u: serves a simple upload HTML page.
    http.HandleFunc("/u", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
            return
        }

        w.Header().Set("Content-Type", "text/html")
        htmlStr := `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>File Upload with Progress</title></head><body><h2>Upload a File</h2><form id="__uploadForm"><input type="file" id="fileInput" name="file"><button type="submit">Upload</button></form><br><div id="__progress"></div><script>let shouldLogThisTime = true;const progressDivTag = document.getElementById('__progress');document.getElementById('__uploadForm').addEventListener('submit', function (event) {event.preventDefault();const fileInput = document.getElementById('fileInput');if (fileInput.files.length === 0) {return;}const file = fileInput.files[0];const formData = new FormData();formData.append('file', file);const xhr = new XMLHttpRequest();xhr.open('POST', '/');xhr.upload.addEventListener('progress', function (e) {if (e.lengthComputable) {if (shouldLogThisTime) {const percentComplete = (e.loaded / e.total) * 100;progressDivTag.innerText = 'Upload progress: $p%'.replace('$p',percentComplete.toFixed(2));}shouldLogThisTime = !shouldLogThisTime;}});xhr.onload = function () {if (xhr.status === 200) {progressDivTag.innerText = "File uploaded successfully.";} else {progressDivTag.innerText = "There was an error during the upload.";}};xhr.send(formData);});</script></body></html>`
        w.Write([]byte(htmlStr))
    })

    // Main handler for "/" endpoint.
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Basic CORS headers.
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

        // Handle preflight OPTIONS requests.
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusOK)
            return
        }

        switch r.Method {
        case http.MethodGet:
            if r.URL.Path == "/" {
                listFilesHandler(w, sharedPath)
            } else {
                // Serve static files from the shared folder.
                filePath := filepath.Join(sharedPath, filepath.Clean(r.URL.Path))
                http.ServeFile(w, r, filePath)
            }
        case http.MethodPost:
            // For file uploads at "/"
            if r.URL.Path == "/" {
                uploadHandler(w, r, sharedPath)
            } else {
                http.NotFound(w, r)
            }
        default:
            w.WriteHeader(http.StatusMethodNotAllowed)
            json.NewEncoder(w).Encode(map[string]string{"error": "Method Not Allowed"})
        }
    })

    addr := fmt.Sprintf("%s:%s", *ipAddr, *port)
    log.Printf("Server running on http://%s\n", addr)
    if err := http.ListenAndServe(addr, nil); err != nil {
        log.Fatalf("Server error: %v", err)
    }
}

// listFilesHandler returns a JSON list of files from the shared folder.
func listFilesHandler(w http.ResponseWriter, sharedPath string) {
    entries, err := os.ReadDir(sharedPath)
    if err != nil {
        http.Error(w, fmt.Sprintf("Unable to list files: %v", err), http.StatusInternalServerError)
        return
    }

    var files []string
    for _, entry := range entries {
        if !entry.IsDir() {
            files = append(files, entry.Name())
        }
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{"files": files})
}

// uploadHandler processes the file upload using a streaming approach (without size limit).
func uploadHandler(w http.ResponseWriter, r *http.Request, sharedPath string) {
    // Use MultipartReader to stream the file data without an artificial size limit.
    mr, err := r.MultipartReader()
    if err != nil {
        http.Error(w, fmt.Sprintf("Error reading multipart data: %v", err), http.StatusBadRequest)
        return
    }

    var fileFound bool
    var fileName string
    for {
        part, err := mr.NextPart()
        if err == io.EOF {
            break
        }
        if err != nil {
            http.Error(w, fmt.Sprintf("Error reading multipart section: %v", err), http.StatusInternalServerError)
            return
        }

        // Look for the field named "file".
        if part.FormName() != "file" {
            continue
        }

        fileName = part.FileName()
        if fileName == "" {
            continue
        }

        fileFound = true
        dstPath := filepath.Join(sharedPath, filepath.Base(fileName))
        dst, err := os.Create(dstPath)
        if err != nil {
            http.Error(w, fmt.Sprintf("Error creating file: %v", err), http.StatusInternalServerError)
            return
        }

        if _, err := io.Copy(dst, part); err != nil {
            dst.Close()
            http.Error(w, fmt.Sprintf("Error saving file: %v", err), http.StatusInternalServerError)
            return
        }
        dst.Close()
        break // Process only the first file part.
    }

    if !fileFound {
        http.Error(w, "No file uploaded", http.StatusBadRequest)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "message":  "File uploaded successfully",
        "filename": fileName,
    })
}

