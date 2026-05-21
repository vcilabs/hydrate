package main

import (
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/pkg/errors"
	"github.com/vcilabs/hydrate"
)

var (
	flags    = flag.NewFlagSet("hydrate", flag.ExitOnError)
	region   = flags.String("region", "", "AWS region (defaults to $AWS_DEFAULT_REGION)")
	basePath = flags.String("path", "", "base path for AWS SSM Parameter Store parameters")
	format   = flags.String("format", "yaml", "input file format: json, yaml, toml (default yaml)")
	debug    = flags.Bool("debug", false, "print debug info to stderr")
	k8s      = flags.Bool("k8s", false, "hydrate Kubernetes Secret/ConfigMap objects' base64-encoded data fields")

	usage = errors.New(`hydrate:

Hydrate	JSON, YAML, TOML config files.

Replace all matching values with strings/secrets from AWS SSM Param Store.
    1. "$SECRET:/custom/parameter/path"
    2. "$$"
    3. "$SECRET"

Usage:
    # Hydrate JSON file:
        hydrate no-secrets.json > secrets.json

    # Hydrate YAML data from stdin:
        echo "data: $SECRET:/app/sit1/app_secret_data_key" | hydrate --format=yml - > secret.yml

	# Hydrate Kubernetes Secrets/ConfigMap data files/values
	# (both "data" and "stringData" fields, handles base64 encoding automatically):
        hydrate -k8s k8s-secret.yml | kubectl apply -

Example:

    Parameter Store:
        PATH                     VALUE
        /custom/parameter/path   a
        /prefix/db_passwd        bb
        /prefix/db_pwd           ccc
    
	input.json:
		{
			"db_password": "$SECRET:/custom/parameter/path",
			"db_passwd": "$$",
			"db_pwd": "$SECRET",
		}
	
	Run command:
        hydrate --path=/prefix input.json > output.json

    output.json:
		{
			"db_password": "a",
			"db_passwd": "bb",
			"db_pwd": "ccc",
		}
`)
)

func main() {
	flags.Parse(os.Args[1:])

	if *region == "" {
		*region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if *region == "" {
		log.Fatal(errors.New("hydrate: --region=[us-west-2] or $AWS_DEFAULT_REGION must be provided"))
	}

	args := flags.Args()
	if len(args) != 1 {
		log.Fatal(usage)
	}
	filename := args[0]

	var r io.Reader
	if filename == "-" {
		if *format == "" {
			log.Fatal(errors.New("hydrate: --format=[json|yaml|toml] must be provided when using STDIN"))
		}
		r = os.Stdin
	} else {
		if *format == "" {
			*format = strings.TrimLeft(filepath.Ext(filename), ".")
		}
		f, err := os.Open(filename)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		r = io.Reader(f)
	}

	sess, err := session.NewSession(&aws.Config{
		CredentialsChainVerboseErrors: aws.Bool(true),
		Region:                        region,
	})
	if err != nil {
		log.Fatal(errors.Wrap(err, "failed to create aws session"))
	}

	paramStore := hydrate.ParamStore(ssm.New(sess, aws.NewConfig()), *basePath)
	if err := paramStore.Hydrate(os.Stdout, r, *format, *k8s); err != nil {
		log.Fatal(err)
	}
}
