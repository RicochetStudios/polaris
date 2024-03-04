package scheme

import (
	polarisv1 "github.com/RicochetStudios/polaris/apis/v1"

	runtime "k8s.io/apimachinery/pkg/runtime"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
)

var Scheme = polarisv1.GlobalScheme
var Codecs = serializer.NewCodecFactory(Scheme)
var ParameterCodec = runtime.NewParameterCodec(Scheme)
