terraform {
	required_version = ">= 1.12.1"

	required_providers {
		aws = {
			source = "hashicorp/aws"
			version = ">= 6.9.0"
		}
	}
}

provider "aws" {
	region = "us-east-1"
}

resource "null_resource" "sam_metadata_aws_lambda_function_event-registration" {
    triggers = {
        DockerTag: "v1"
        DockerContext: "./app"
        Dockerfile: Dockerfile
    }
    depends_on = [
        
    ]
}

resource "aws_lambda_function" "event-registration" {
    function_name = "event-registration"
    package_type = "Image"
    image_uri = "icaa-event-registration:latest"
    role = aws_iam_role.event-registration.arn
    timeout = 30
    memory_size = 256

    architectures = ["arm64"]

    depends_on = [
        null_resource.build_lambda_function
    ]	
}

resource "aws_iam_role" "event-registration" {
  name = "event-registration"

  assume_role_policy = <<EOF
    {
    "Version": "2012-10-17",
    "Statement": [
        {
        "Action": "sts:AssumeRole",
        "Principal": {
            "Service": "lambda.amazonaws.com"
        },
        "Effect": "Allow",
        "Sid": ""
        }
    ]
    }
    EOF

}
