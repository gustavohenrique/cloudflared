package simpleserver

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/urfave/cli/v2"
)

type Config struct {
	Port      int
	MaxSize   int
	UploadDir string
}

type Server struct {
	config Config
}

func Flags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:    "port",
			Value:   8080,
			Usage:   "HTTP Server port number",
		},
		&cli.IntFlag{
			Name:    "maxsize",
			Value:   100,
			Usage:   "Max upload file size in MB",
		},
		&cli.StringFlag{
			Name:    "upload-dir",
			Value:   "",
			Usage:   "Directory for uploads",
		},
	}
}

func New(config Config) *Server {
	return &Server{config: config}
}

func WithCtx(c *cli.Context) *Server {
	config := Config{
		Port:      c.Int("port"),
		MaxSize:   c.Int("size"),
		UploadDir: c.String("upload-dir"),
	}
	return New(config)
}

func (s *Server) Start() error {
	var config = s.config
	e := echo.New()
	e.Debug = false
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())
	e.Pre(middleware.RemoveTrailingSlash())
	var maxSize = 100
	if config.MaxSize > 0 {
		maxSize = config.MaxSize
	}
	e.Use(middleware.BodyLimit(fmt.Sprintf("%dM", maxSize)))
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
	}))

	e.GET("/favicon.ico", s.handleFavicon)
	e.PUT("*", s.handleUpload)
	e.GET("/:dir/:filename", s.handleDownload)

	var port = 8080
	if config.Port > 0 {
		port = config.Port
	}
	fmt.Printf("Server starting on port %d...\n", port)
	return e.Start(fmt.Sprintf(":%d", port))
}

func (s *Server) handleUpload(c echo.Context) error {
	var dir = base58(6)
	var uploadDir = filepath.Join(s.getUploadDir(), dir)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return c.String(http.StatusInternalServerError, "Failed to create upload directory")
	}

	filename := filepath.Base(c.Request().URL.Path)
	if filename == "" {
		filename = "uploaded-file"
	}

	path := filepath.Join(uploadDir, filename)
	file, err := os.Create(path)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to create file")
	}
	defer file.Close()

	_, err = io.Copy(file, c.Request().Body)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to save file")
	}

	downloadURL := fmt.Sprintf("http://%s/%s/%s", c.Request().Host, dir, filename)
	return c.String(http.StatusCreated, fmt.Sprintf("File uploaded successfully. Download at:\n%s\n", downloadURL))
}

func (s *Server) handleDownload(c echo.Context) error {
	dir := c.Param("dir")
	filename := c.Param("filename")
	path := filepath.Join(s.getUploadDir(), dir, filename)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return c.String(http.StatusNotFound, "File not found")
	}

	return c.File(path)
}

func (s *Server) handleFavicon(c echo.Context) error {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		<rect width="100" height="100" fill="#4a90e2"/>
		<circle cx="50" cy="50" r="40" fill="#fff"/>
	</svg>`
	return c.Blob(http.StatusOK, "image/svg+xml", []byte(svg))
}

func (s *Server) getUploadDir() string {
	uploadDir := s.config.UploadDir
	if uploadDir == "" {
		uploadDir = filepath.Join(os.TempDir(), "uploads")
	}
	return uploadDir
}

func base58(size int) string {
	const (
		alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	)

	var id = make([]byte, size)
	if _, err := rand.Read(id); err != nil {
		return "0"
	}
	for i, p := range id {
		id[i] = alphabet[int(p)%len(alphabet)] // discard everything but the least significant bits
	}
	return string(id)
}

func waitForSignal(graceShutdownC chan struct{}) {
	signals := make(chan os.Signal, 10)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)

	select {
	case s := <-signals:
		log.Printf("Initiating graceful shutdown due to signal %s ...\n", s)
		close(graceShutdownC)
	case <-graceShutdownC:
	}
}
