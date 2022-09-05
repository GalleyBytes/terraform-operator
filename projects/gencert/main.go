package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/isaaguilar/selfsigned"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var path = "/conversion"

var (
	namespace   string
	serviceName string
	secretName  string
	kubeconfig  string
)

func init() {
	kubeconfig = os.Getenv("KUBECONFIG")
	namespace = os.Getenv("NAMESPACE")
	serviceName = os.Getenv("SERVICE")
	secretName = os.Getenv("SECRET")
	if namespace == "" {
		log.Fatal("NAMESPACE is required")
	}
	if serviceName == "" {
		log.Fatal("SERVICE is required")
	}
	if secretName == "" {
		log.Fatal("SECRET is required")
	}
}

// GetClientsOrDie returns 2 clients. Core k8s and api-extensions for CRDs.
func GetClientsOrDie(kubeconfigPath string) (kubernetes.Interface, apiextensionsclient.Interface) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		log.Fatal("Failed to get config for clientset")
	}

	return kubernetes.NewForConfigOrDie(config), apiextensionsclient.NewForConfigOrDie(config)
}

// isCertValid checks cert expiration and cert dns names
func isCertValid(selfSignedCert selfsigned.SelfSignedCert, dnsNames []string) bool {
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM(selfSignedCert.CACert)
	if !ok {
		log.Println("failed to parse root certificate")
		return false
	}

	block, _ := pem.Decode(selfSignedCert.TLSCert)
	if block == nil {
		log.Println("failed to parse certificate PEM")
		return false
	}
	cert, xerr := x509.ParseCertificate(block.Bytes)
	if xerr != nil {
		log.Println("failed to parse certificate: " + xerr.Error())
		return false
	}

	t := time.Now()
	opts := x509.VerifyOptions{
		CurrentTime: t.Add(24 * 30 * time.Hour), // 30 days
		DNSName:     dnsNames[0],
		Roots:       roots,
	}

	if _, err := cert.Verify(opts); err != nil {
		log.Println("failed to verify certificate: " + err.Error())
		return false
	}
	return true
}

func main() {
	ctx := context.TODO()
	dnsNames := []string{
		fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, namespace),
		fmt.Sprintf("%s.%s.svc", serviceName, namespace),
		fmt.Sprintf("%s.%s", serviceName, namespace),
	}
	selfSignedCert := selfsigned.NewSelfSignedCertOrDie(dnsNames)
	clientset, apiclientset := GetClientsOrDie(kubeconfig)
	secretClient := clientset.CoreV1().Secrets(namespace)

	secret, err := secretClient.Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			secret, err = secretClient.Create(
				ctx,
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: namespace,
					},
					Type: corev1.SecretTypeTLS,
					Data: map[string][]byte{
						"ca.crt":  selfSignedCert.CACert,
						"tls.crt": selfSignedCert.TLSCert,
						"tls.key": selfSignedCert.TLSKey,
					},
				},
				metav1.CreateOptions{},
			)
			if err != nil {
				log.Panic(err)
			}
			log.Printf("TLS certs in secret/%s created\n", secret.Name)
		} else {
			log.Panic(err)
		}
	} else {
		if isCertValid(selfsigned.SelfSignedCert{CACert: secret.Data["ca.crt"], TLSCert: secret.Data["tls.crt"], TLSKey: secret.Data["tls.key"]}, dnsNames) {
			log.Printf("TLS certs in secret/%s are still valid", secret.Name)
		} else {
			secret.Data["ca.crt"] = selfSignedCert.CACert
			secret.Data["tls.crt"] = selfSignedCert.TLSCert
			secret.Data["tls.key"] = selfSignedCert.TLSKey
			secret, err = secretClient.Update(ctx, secret, metav1.UpdateOptions{})
			if err != nil {
				log.Panic(err)
			}
			log.Printf("TLS certs in secret/%s updated\n", secret.Name)
		}
	}

	crdClient := apiclientset.ApiextensionsV1().CustomResourceDefinitions()
	crd, err := crdClient.Get(ctx, "terraforms.tf.isaaguilar.com", metav1.GetOptions{})
	if err != nil {
		log.Panic(err)
	}

	if crd.Spec.Conversion != nil {
		if crd.Spec.Conversion.Webhook != nil {
			if crd.Spec.Conversion.Webhook.ClientConfig != nil {
				if string(crd.Spec.Conversion.Webhook.ClientConfig.CABundle) == string(secret.Data["ca.crt"]) {
					log.Println("CRD CABundle is up to date")
					return
				}
			}
		}
	}

	crd.Spec.Conversion = &apiextensionsv1.CustomResourceConversion{
		Strategy: apiextensionsv1.WebhookConverter,
		Webhook: &apiextensionsv1.WebhookConversion{
			ConversionReviewVersions: []string{"v1"},
			ClientConfig: &apiextensionsv1.WebhookClientConfig{
				Service: &apiextensionsv1.ServiceReference{
					Namespace: namespace,
					Name:      serviceName,
					Path:      &path,
				},
				CABundle: secret.Data["ca.crt"],
			},
		},
	}
	_, err = crdClient.Update(ctx, crd, metav1.UpdateOptions{})
	if err != nil {
		log.Panic(err)
	}
	log.Println("CRD CABundle is updated")

}
