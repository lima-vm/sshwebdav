package sshwebdav

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	iofs "io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"

	"github.com/pkg/sftp"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/webdav"
)

type Opt func(*SSHWebDAV) error

func WithSSHConfig(sshConfig string) Opt {
	return func(x *SSHWebDAV) error {
		x.sshConfig = sshConfig
		return nil
	}
}

func WithSSHIdentity(sshIdentity string) Opt {
	return func(x *SSHWebDAV) error {
		x.sshIdentity = sshIdentity
		return nil
	}
}

func WithSSHOptions(sshOptions []string) Opt {
	return func(x *SSHWebDAV) error {
		x.sshOptions = sshOptions
		return nil
	}
}

func New(sshURL, webdavURL *url.URL, opts ...Opt) (*SSHWebDAV, error) {
	if sshURL.Scheme != "ssh" {
		return nil, fmt.Errorf("expected ssh url, got %q", sshURL.String())
	}
	if webdavURL.Scheme != "http" {
		// TODO: https
		return nil, fmt.Errorf("expected http url, got %q", webdavURL.String())
	}

	x := &SSHWebDAV{
		sshURL:    sshURL,
		webdavURL: webdavURL,
	}
	for _, f := range opts {
		if err := f(x); err != nil {
			return nil, err
		}
	}
	return x, nil
}

type SSHWebDAV struct {
	sshURL      *url.URL
	webdavURL   *url.URL
	sshConfig   string
	sshIdentity string
	sshOptions  []string
}

func (x *SSHWebDAV) Serve() error {
	sshArgs, err := sshArgs(x.sshURL, x.sshConfig, x.sshIdentity, x.sshOptions)
	if err != nil {
		return err
	}
	sshCmd := exec.Command("ssh", sshArgs...)
	sshCmd.Stderr = os.Stderr
	sshW, err := sshCmd.StdinPipe()
	if err != nil {
		return err
	}
	sshR, err := sshCmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := sshCmd.Start(); err != nil {
		return err
	}
	defer func() { _ = sshCmd.Wait() }()
	sftpClient, err := sftp.NewClientPipe(sshR, sshW)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	fs, ls, err := NewFileSystem(sftpClient, x.sshURL.Path)
	if err != nil {
		return err
	}

	handler := &webdav.Handler{
		Prefix:     x.webdavURL.Path,
		FileSystem: fs,
		LockSystem: ls,
		Logger: func(r *http.Request, e error) {
			if e != nil {
				logrus.WithField("request", r).WithError(e).Errorf("handler: %+v", e)
			} else {
				logrus.WithField("request", r).WithError(e).Debug("handler")
			}
		},
	}

	srv := &http.Server{
		Addr:    x.webdavURL.Host,
		Handler: handler,
	}
	defer srv.Close()

	return srv.ListenAndServe()
}

func sshArgs(sshURL *url.URL, sshConfig, sshIdentity string, sshOptions []string) ([]string, error) {
	if sshURL.Scheme != "ssh" {
		return nil, fmt.Errorf("expected ssh url, got %q", sshURL.String())
	}
	var args []string
	if sshConfig != "" {
		args = append(args, "-F", sshConfig)
	}
	if sshIdentity != "" {
		args = append(args, "-i", sshIdentity)
	}
	for _, f := range sshOptions {
		args = append(args, "-o", f)
	}
	if u := sshURL.User; u != nil {
		if _, ok := u.Password(); ok {
			return nil, fmt.Errorf("plain-text SSH passsword is not supported, use public key")
		}
		if username := u.Username(); username != "" {
			args = append(args, "-l", username)
		}
	}
	host := sshURL.Host
	if h, p, err := net.SplitHostPort(sshURL.Host); err == nil {
		args = append(args, "-p", p)
		host = h
	}
	args = append(args, host)
	args = append(args, "-s", "sftp")
	return args, nil
}

func NewFileSystem(sftpClient *sftp.Client, root string) (webdav.FileSystem, webdav.LockSystem, error) {
	fs := &fileSystem{
		sftpClient: sftpClient,
		root:       path.Clean(root),
	}
	ls := webdav.NewMemLS()
	return fs, ls, nil
}

type fileSystem struct {
	sftpClient *sftp.Client // thread-safe
	root       string
}

var ErrReadOnlyFS = errors.New("read-only file system")

func (fs *fileSystem) Mkdir(_ context.Context, name string, perm os.FileMode) (retErr error) {
	logrus.WithField("name", name).Debug("> *fileSystem.Mkdir")
	defer func() {
		logrus.WithField("name", name).WithError(retErr).Debug("< *fileSystem.Mkdir")
	}()
	return ErrReadOnlyFS
}

func (fs *fileSystem) OpenFile(_ context.Context, name string, flag int, perm os.FileMode) (res webdav.File, retErr error) {
	logrus.WithField("name", name).WithField("flag", flag).WithField("perm", perm).Debug("> *fileSystem.OpenFile")
	defer func() {
		logrus.WithField("name", name).WithField("flag", flag).WithField("perm", perm).WithError(retErr).Debug("< *fileSystem.OpenFile")
	}()
	remotePath := fs.remotePath(name)
	st, err := fs.sftpClient.Stat(remotePath)
	if err != nil {
		// even on err we need to return a file without err, otherwise `ls` result becomes empty
		return &errFile{
			err: err,
			st: func() (iofs.FileInfo, error) {
				return st, err
			}}, nil
	}

	if st.Mode()&(iofs.ModeNamedPipe|iofs.ModeDevice|iofs.ModeCharDevice) != 0 {
		// don't open pipe, it hangs
		return &errFile{
			err: fmt.Errorf("can't be opened"),
			st: func() (iofs.FileInfo, error) {
				return st, nil
			}}, nil
	}

	f, err := fs.sftpClient.Open(remotePath) // read-only
	if err != nil {
		// even on err we need to return a file without err, otherwise `ls` result becomes empty
		return &errFile{
			err: err,
			st: func() (iofs.FileInfo, error) {
				return st, nil
			}}, nil
	}
	ff := &file{
		File:       f,
		sftpClient: fs.sftpClient,
		remotePath: remotePath,
	}
	return ff, nil
}

func (fs *fileSystem) RemoveAll(_ context.Context, name string) (retErr error) {
	logrus.WithField("name", name).Debug("> *fileSystem.RemoveAll")
	defer func() {
		logrus.WithField("name", name).WithError(retErr).Debug("< *fileSystem.RemoveAll")
	}()
	return ErrReadOnlyFS
}

func (fs *fileSystem) Rename(_ context.Context, oldName, newName string) (retErr error) {
	logrus.WithField("oldName", oldName).WithField("newName", newName).Debug("> *fileSystem.Rename")
	defer func() {
		logrus.WithField("oldName", oldName).WithField("newName", newName).WithError(retErr).Debug("< *fileSystem.Rename")
	}()
	return ErrReadOnlyFS
}

func (fs *fileSystem) Stat(_ context.Context, name string) (fi os.FileInfo, retErr error) {
	logrus.WithField("name", name).Debug("> *fileSystem.Stat")
	defer func() {
		logrus.WithField("name", name).WithField("fi", fi).WithError(retErr).Debug("< *fileSystem.Stat")
	}()
	return fs.sftpClient.Stat(fs.remotePath(name))
}

func (fs *fileSystem) remotePath(name string) string {
	remotePath := path.Join(fs.root, path.Clean(name))
	//	logrus.Debugf("remotePath(%q) => %q", name, remotePath)
	return remotePath
}

type file struct {
	*sftp.File
	sftpClient *sftp.Client
	remotePath string
}

func (f *file) Readdir(count int) (fis []iofs.FileInfo, retErr error) {
	logrus.WithField("count", count).WithField("f.remotePath", f.remotePath).Debug("> *file.Readdir")
	defer func() {
		logrus.WithField("count", count).WithField("f.remotePath", f.remotePath).WithField("nfis", len(fis)).WithError(retErr).Debug("< *file.Readdir")
	}()
	if count > 0 {
		return nil, fmt.Errorf("unsupported call: Readdir(%d)", count)
	}
	return f.sftpClient.ReadDir(f.remotePath)
}

func (f *file) Stat() (fi fs.FileInfo, retErr error) {
	logrus.Debug("> *file.Stat")
	defer func() {
		logrus.WithField("fi", fi).WithError(retErr).Debug("< *file.Stat")
	}()
	return f.File.Stat()
}

type errFile struct {
	err error
	st  func() (iofs.FileInfo, error)
}

func (f *errFile) Close() error {
	return nil
}

func (f *errFile) Read(p []byte) (n int, err error) {
	return 0, f.err
}

func (f *errFile) Seek(offset int64, whence int) (int64, error) {
	return 0, f.err
}

func (f *errFile) Readdir(count int) (fis []iofs.FileInfo, retErr error) {
	logrus.WithField("count", count).Debug("> *errFile.Readdir")
	defer func() {
		logrus.WithField("count", count).WithError(retErr).Debug("< *errFile.Readdir")
	}()
	return nil, f.err
}

func (f *errFile) Stat() (fi fs.FileInfo, retErr error) {
	logrus.Debug("> *errFile.Stat")
	defer func() {
		logrus.WithField("fi", fi).WithError(retErr).Debug("< *errFile.Stat")
	}()
	return f.st()
}

func (f *errFile) Write(p []byte) (n int, err error) {
	return 0, f.err
}
