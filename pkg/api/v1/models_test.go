package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// validMetadata is the shape the adapter actually serves; each subtest below
// breaks exactly one field of it so a failure names the rule that regressed.
func validMetadata() *OptimizerAdapterMetadata {
	return &OptimizerAdapterMetadata{
		Optimizer: &Optimizer{
			Name:    "sleeko",
			Vendor:  "sleeko",
			Version: "1.0.0",
		},
		Capabilities: []*OptimizerCapability{{
			ConsumesMimeTypes: []string{MimeTypeOCIIndex, MimeTypeDockerManifestList},
			ProducesMimeTypes: []string{MimeTypeOptimizationReport},
		}},
		Properties: OptimizerProperties{
			"harbor.optimizer-adapter/registry-authorization-type": "Basic",
		},
	}
}

func TestOptimizerAdapterMetadata_Validate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(md *OptimizerAdapterMetadata)
		wantErr string
	}{
		{"valid", func(*OptimizerAdapterMetadata) {}, ""},
		{
			"nil optimizer",
			func(md *OptimizerAdapterMetadata) { md.Optimizer = nil },
			"invalid optimizer in metadata",
		},
		{
			"empty name",
			func(md *OptimizerAdapterMetadata) { md.Optimizer.Name = "" },
			"invalid optimizer in metadata",
		},
		{
			"empty version",
			func(md *OptimizerAdapterMetadata) { md.Optimizer.Version = "" },
			"invalid optimizer in metadata",
		},
		{
			"empty vendor",
			func(md *OptimizerAdapterMetadata) { md.Optimizer.Vendor = "" },
			"invalid optimizer in metadata",
		},
		{
			"no capabilities",
			func(md *OptimizerAdapterMetadata) { md.Capabilities = nil },
			"invalid capabilities in metadata",
		},
		{
			"capability consumes nothing",
			func(md *OptimizerAdapterMetadata) { md.Capabilities[0].ConsumesMimeTypes = nil },
			"missing consumes_mime_types in capability",
		},
		{
			"capability does not produce a report",
			func(md *OptimizerAdapterMetadata) {
				md.Capabilities[0].ProducesMimeTypes = []string{"application/json"}
			},
			"missing " + MimeTypeOptimizationReport + " in produces_mime_types",
		},
		{
			// Every capability is checked, not just the first.
			"second capability is invalid",
			func(md *OptimizerAdapterMetadata) {
				md.Capabilities = append(md.Capabilities, &OptimizerCapability{
					ConsumesMimeTypes: []string{MimeTypeOCIArtifact},
					ProducesMimeTypes: nil,
				})
			},
			"missing " + MimeTypeOptimizationReport + " in produces_mime_types",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			md := validMetadata()
			c.mutate(md)

			err := md.Validate()
			if c.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, c.wantErr)
		})
	}
}

func TestOptimizerAdapterMetadata_HasCapability(t *testing.T) {
	md := validMetadata()
	// A mime type listed by a later capability must match too.
	md.Capabilities = append(md.Capabilities, &OptimizerCapability{
		ConsumesMimeTypes: []string{MimeTypeOCIArtifact},
		ProducesMimeTypes: []string{MimeTypeOptimizationReport},
	})

	require.True(t, md.HasCapability(MimeTypeOCIIndex))
	require.True(t, md.HasCapability(MimeTypeDockerManifestList))
	require.True(t, md.HasCapability(MimeTypeOCIArtifact))
	require.False(t, md.HasCapability(MimeTypeDockerArtifact))
	require.False(t, md.HasCapability(""))

	require.False(t, (&OptimizerAdapterMetadata{}).HasCapability(MimeTypeOCIIndex))
}

func TestOptimizeRequest_Validate(t *testing.T) {
	valid := func() *OptimizeRequest {
		return &OptimizeRequest{
			Registry: &Registry{URL: "http://harbor-core:8080", Authorization: "Basic dXNlcjpwYXNz"},
			Artifact: &Artifact{
				Repository: "library/photon",
				Tag:        "5.0",
				Digest:     "sha256:abc",
				MimeType:   MimeTypeOCIIndex,
			},
		}
	}

	cases := []struct {
		name    string
		mutate  func(req *OptimizeRequest)
		wantErr string
	}{
		{"valid", func(*OptimizeRequest) {}, ""},
		{
			// Harbor omits the scan report for an unscanned artifact; that is a
			// worker-level terminal report, not a request validation error.
			"valid without a scan report",
			func(req *OptimizeRequest) { req.ScanReport = "" },
			"",
		},
		{
			// A tag is optional: the adapter pulls by digest.
			"valid without a tag",
			func(req *OptimizeRequest) { req.Artifact.Tag = "" },
			"",
		},
		{
			"nil registry",
			func(req *OptimizeRequest) { req.Registry = nil },
			"optimize request: invalid registry",
		},
		{
			"empty registry url",
			func(req *OptimizeRequest) { req.Registry.URL = "" },
			"optimize request: invalid registry",
		},
		{
			"nil artifact",
			func(req *OptimizeRequest) { req.Artifact = nil },
			"optimize request: invalid artifact",
		},
		{
			"empty digest",
			func(req *OptimizeRequest) { req.Artifact.Digest = "" },
			"optimize request: invalid artifact",
		},
		{
			"empty repository",
			func(req *OptimizeRequest) { req.Artifact.Repository = "" },
			"optimize request: invalid artifact",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := valid()
			c.mutate(req)

			err := req.Validate()
			if c.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, c.wantErr)
		})
	}
}

func TestErrorResponse_Error(t *testing.T) {
	er := &ErrorResponse{Err: &Error{Message: "decode optimize request: unexpected EOF"}}
	require.EqualError(t, er, "decode optimize request: unexpected EOF")

	require.EqualError(t, &ErrorResponse{}, "nil error")
}
