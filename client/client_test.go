package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeDocker sobe um "Docker" de mentira e devolve um Client apontando para ele.
func fakeDocker(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	c, err := newClient("tcp://" + strings.TrimPrefix(ts.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestRotasUsamVersaoDaAPI(t *testing.T) {
	var path string
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Write([]byte(`[]`))
	})
	if _, err := c.ListContainers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/"+apiVersion+"/containers/json" {
		t.Fatalf("rota = %q", path)
	}
}

func TestErrosTestaveis(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No such container: x"}`))
	})
	_, err := c.InspectContainer(context.Background(), "x")
	if !IsNotFound(err) || IsConflict(err) {
		t.Fatalf("esperava 404, veio %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "No such container: x" {
		t.Fatalf("APIError = %+v", apiErr)
	}
}

func TestPathIDRecusaRotaInvalida(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("não devia chamar o Docker: %s", r.URL.Path)
	})
	for _, id := range []string{"", "  ", ".", ".."} {
		if err := c.StartContainer(context.Background(), id); err == nil {
			t.Errorf("id %q devia dar erro", id)
		}
	}
}

func TestPathIDEscapaBarra(t *testing.T) {
	var raw string
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		raw = r.URL.RawPath
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.StartContainer(context.Background(), "../images"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "..%2Fimages") {
		t.Fatalf("barra não foi escapada: %q", raw)
	}
}

func TestCreateContainerBaixaImagemQuandoFalta(t *testing.T) {
	var chamadas []string
	criou := false
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		chamadas = append(chamadas, r.Method+" "+strings.TrimPrefix(r.URL.Path, "/"+apiVersion))
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/create") && !criou:
			criou = true
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"No such image"}`))
		case strings.HasSuffix(r.URL.Path, "/images/create"):
			if r.URL.Query().Get("tag") != "latest" {
				t.Errorf("pull sem tag latest: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"status":"ok"}`))
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			var body CreateContainer
			json.NewDecoder(r.Body).Decode(&body)
			if len(body.HostConfig.DNS) == 0 {
				t.Error("DNS padrão não foi aplicado")
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"Id":"abc123","Warnings":[]}`))
		}
	})

	hc := &HostConfig{}
	out, err := c.CreateContainer(context.Background(), "mc", CreateContainer{Image: "nginx", HostConfig: hc})
	if err != nil || out.ID != "abc123" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if len(chamadas) != 3 {
		t.Fatalf("chamadas = %v", chamadas)
	}
	if len(hc.DNS) != 0 {
		t.Error("ensureDNS alterou o HostConfig de quem chamou")
	}
}

func TestPullImageErroNoMeioDoDownload(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"baixando"}{"error":"disco cheio"}`))
	})
	err := c.PullImage(context.Background(), "nginx:1.25")
	if err == nil || !strings.Contains(err.Error(), "disco cheio") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewClientDockerHost(t *testing.T) {
	casos := map[string]bool{ // host -> válido
		"tcp://localhost:2375":        true,
		"https://10.0.0.1:2376":       true,
		"unix:///var/run/docker.sock": true,
		"tcp://":                      false,
		"npipe:////./pipe/docker":     false,
		"ftp://x":                     false,
	}
	for host, ok := range casos {
		c, err := newClient(host)
		if (err == nil) != ok {
			t.Errorf("%s: err = %v", host, err)
		}
		if ok && c.DaemonHost() != host {
			t.Errorf("%s: DaemonHost = %q", host, c.DaemonHost())
		}
	}
}

func TestHasTagOrDigest(t *testing.T) {
	casos := map[string]bool{
		"nginx": false, "nginx:1.25": true, "nginx@sha256:abc": true,
		"localhost:5000/app": false, "localhost:5000/app:v1": true,
	}
	for img, want := range casos {
		if hasTagOrDigest(img) != want {
			t.Errorf("%s: want %v", img, want)
		}
	}
}

// frame monta um pedaço do stream multiplexado do Docker (stdout = 1).
func frame(s string) []byte {
	h := make([]byte, 8)
	h[0] = 1
	h[4], h[5], h[6], h[7] = byte(len(s)>>24), byte(len(s)>>16), byte(len(s)>>8), byte(len(s))
	return append(h, s...)
}

func TestTipoContainer(t *testing.T) {
	casos := map[string]string{ // entrada -> tipo ("" = erro)
		"3f2a9c1b7d4e": "id", "/mc": "nome", "mc-exemplo": "nome",
		"": "", "  ": "", "../images": "", "-rf": "", "a b": "",
	}
	for in, want := range casos {
		got, err := tipoContainer(in)
		if got != want || (want == "") != (err != nil) {
			t.Errorf("%q: got %q err=%v, want %q", in, got, err, want)
		}
	}
}

func TestLogsContainerSemFollow(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/json") {
			w.Write([]byte(`{"Config":{"Tty":false}}`))
			return
		}
		if r.URL.Query().Has("follow") {
			t.Error("LogsContainer não pode mandar follow (travaria até o timeout)")
		}
		w.Write(frame("linha 1\n"))
		w.Write(frame("linha 2\n"))
	})
	got, err := c.LogsContainer(context.Background(), "/mc")
	if err != nil || got != "linha 1\nlinha 2\n" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestStreamLogsAoVivo(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/json") {
			w.Write([]byte(`{"Config":{"Tty":false}}`))
			return
		}
		q := r.URL.Query()
		if q.Get("follow") != "true" || q.Get("tail") != "200" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		// Manda duas linhas e fica aberto, como o Docker com follow=true.
		w.Write(frame("antiga\n"))
		w.(http.Flusher).Flush()
		w.Write(frame("nova\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- c.StreamLogs(ctx, "mc", 200, pw); pw.Close() }()

	buf := make([]byte, 64)
	var got string
	for !strings.Contains(got, "nova\n") {
		n, err := pr.Read(buf)
		if err != nil {
			t.Fatalf("leu %q, err=%v", got, err)
		}
		got += string(buf[:n])
	}
	if got != "antiga\nnova\n" {
		t.Fatalf("got %q", got)
	}

	cancel() // usuário fechou a página
	go io.Copy(io.Discard, pr)
	if err := <-done; err != nil {
		t.Fatalf("cancelar o ctx devia devolver nil, veio %v", err)
	}
}
