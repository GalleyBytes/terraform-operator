package admission

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"github.com/isaaguilar/terraform-operator/pkg/webhook/admission/convert"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unstructuredv1 "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

type ConversionWebhook struct {
	log logr.Logger
}

func NewConversionWebhook(log logr.Logger) ConversionWebhook {
	return ConversionWebhook{log: log}
}

func (c ConversionWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := c.log
	convertReview := &apiextensionsv1.ConversionReview{}
	err := json.NewDecoder(r.Body).Decode(convertReview)
	if err != nil {
		logger.Error(err, "failed to read conversion request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	response, status := c.converter(convertReview.Request)
	convertReview.Response = response
	w.WriteHeader(status)
	if status == 200 {
		log.Printf("Successfully converted resource(s): %s", objectNames(convertReview))
	} else {
		log.Printf("Failed to convert resource(s) %s: %s", objectNames(convertReview), response.Result.Status)
	}
	b, _ := json.Marshal(convertReview)
	w.Write(b)
}

func objectNames(review *apiextensionsv1.ConversionReview) string {
	names := []string{}
	for _, obj := range review.Response.ConvertedObjects {
		unstructured := unstructuredv1.Unstructured{}
		err := json.Unmarshal(obj.Raw, &unstructured)
		if err == nil {
			names = append(names, unstructured.GetName())
		}
	}
	return strings.Join(names, ",")
}

// helper to construct error response.
func errored(uid types.UID, err error) (*apiextensionsv1.ConversionResponse, int) {
	return &apiextensionsv1.ConversionResponse{
		UID: uid,
		Result: metav1.Status{
			Status:  metav1.StatusFailure,
			Message: err.Error(),
		},
	}, http.StatusBadRequest
}

// Takes a conversionRequest and always returns a conversionResponse.
func (c ConversionWebhook) converter(request *apiextensionsv1.ConversionRequest) (*apiextensionsv1.ConversionResponse, int) {
	desiredAPIVersion := request.DesiredAPIVersion
	if desiredAPIVersion == "" {
		return errored(request.UID, fmt.Errorf("conversion request did not have a desired api version"))
	}
	responseObjects := make([]runtime.RawExtension, len(request.Objects))
	for i, obj := range request.Objects {
		unstructured := unstructuredv1.Unstructured{}
		err := json.Unmarshal(obj.Raw, &unstructured)
		if err != nil {
			return errored(request.UID, err)
		}
		haveAPIVersion := unstructured.GetAPIVersion()
		if desiredAPIVersion == haveAPIVersion {
			return errored(request.UID, fmt.Errorf("conversion from a version to itself should not call the webhook: %s", haveAPIVersion))
		}

		var (
			raw        []byte
			object     runtime.Object
			convertErr error
		)
		switch desiredAPIVersion {
		case "tf.isaaguilar.com/v1alpha1":
			switch haveAPIVersion {
			case "tf.isaaguilar.com/v1alpha2":
				raw, object, convertErr = convert.ConvertV1alpha2ToV1alpha1(obj.Raw)
			default:
				return errored(request.UID, fmt.Errorf("unexpected conversion version %s", haveAPIVersion))

			}
		case "tf.isaaguilar.com/v1alpha2":
			switch haveAPIVersion {
			case "tf.isaaguilar.com/v1alpha1":
				raw, object, convertErr = convert.ConvertV1alpha1ToV1alpha2(obj.Raw)
			default:
				return errored(request.UID, fmt.Errorf("unexpected conversion version %s", haveAPIVersion))
			}
		default:
			return errored(request.UID, fmt.Errorf("unexpected desired version %s", desiredAPIVersion))
		}

		if convertErr != nil {
			return errored(request.UID, fmt.Errorf("error in conversion from %s to %s: %s", desiredAPIVersion, haveAPIVersion, convertErr))
		}
		responseObjects[i] = runtime.RawExtension{
			Raw:    raw,
			Object: object,
		}
	}

	return &apiextensionsv1.ConversionResponse{
		UID:              request.UID,
		ConvertedObjects: responseObjects,
		Result: metav1.Status{
			Status: metav1.StatusSuccess,
		},
	}, http.StatusOK
}
