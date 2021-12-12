package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var lastIP string

// dominio sobre el cual se sejecutaran las  updates en los subdomains
var domain, intervals, secretFile, queryURL string = os.Getenv("DOMAIN"), os.Getenv("INTERVAL"), os.Getenv("SECRETSFILE"), os.Getenv("QUERYURL")

func handleErrr(err error) {
	if err != nil {
		log.Print(err)
	}
}

// Verifica ip en is
func consultIP() (actualIP string, err error) {
	req, err := http.Get(queryURL)
	if err != nil {
		return "", err
	}
	defer req.Body.Close()
	resultBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}
	actualIP = string(resultBytes)
	return actualIP, nil
}

// Funcion que lee el archivo con los datos de cada subdominio
// este archvivo estara almacenado como un secret en k8s mapeado al volumen
func secretReader() (secrets []string, err error) {
	file, err := os.Open(secretFile)
	if err != nil {
		log.Fatal("Error In file Read", err)
	}
	defer file.Close()
	reads, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	// leemos un solo string
	secret := string(reads)
	// dividimos strings por nueva linea
	secrets = strings.Split(secret, "\n")
	return secrets, nil
}

// Funcion que construlle las uri desde una lista de subdominios
func uriConstructor(lista []string) (uris []string, err error) {
	if lista == nil {
		err = fmt.Errorf("empty List Aborting mission")
		return nil, err
	}
	var subDom, user, pass string
	for _, info := range lista {
		data := strings.Split(info, ":")
		subDom, user, pass = data[0], data[1], data[2]
		uris = append(uris, fmt.Sprintf("https://%s:%s@domains.google.com/nic/update?hostname=%s.%s", user, pass, subDom, domain))
	}
	return uris, nil

}

func main() {
	waitInt, err := strconv.Atoi(intervals)
	if err != nil {
		log.Fatal(err)
	}
	if waitInt == 0 {
		log.Fatal("Error Refresh interval incorrect, cant be 0.")
	}
	if domain == "" || secretFile == "" || queryURL == "" || intervals == "" {
		err := fmt.Errorf("error on Init file DOMAIN=%s SECRETFILE= %s QUERYURL=%s INTERVAL=%s", domain, secretFile, queryURL, intervals)
		log.Fatal(err)
	}
	for {
		actualIP, err := consultIP()
		if err != nil {
			log.Print(err)
		}
		// verificamos la ip anterior si cambio o es la misma?
		if actualIP == lastIP {
			log.Printf("IPs => %s == %s", lastIP, actualIP)

		} else {
			// De ser el primer run quiero que corra todo a modo de prueba
			if lastIP == "" {
				lastIP = actualIP
			}
			concurrent := make(chan struct{})
			urls, err := secretReader()
			handleErrr(err)
			uriList, err := uriConstructor(urls)
			handleErrr(err)
			// concurrente ejecutamos los get para los update a modo de prueba quiero ver si alguno falla por lo que veo la response
			for _, url := range uriList {
				go func(url string) {
					resp, err := http.Get(url)
					handleErrr(err)
					defer resp.Body.Close()
					body, err := io.ReadAll(resp.Body)
					handleErrr(err)
					log.Printf("Ip changed, requesting update \n@ %s\nresponse: %s", url, body)
					concurrent <- struct{}{}
				}(url)
			}
			for range uriList {
				<-concurrent
			}
		}
		log.Printf("Wating %d Minutes for next RUN.", waitInt)
		time.Sleep(time.Minute * time.Duration(waitInt))
	}
}
