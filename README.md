# Resume Customizer Service

An AI-powered serverless application that automatically customizes resumes and generates cover letters based on job postings. Built with AWS Lambda, API Gateway, and Amazon Bedrock Nova.

## Features

- **Resume Optimization**: Analyzes job descriptions and tailors your resume to match requirements
- **Cover Letter Generation**: Creates personalized cover letters for each application
- **Change Summary**: Provides detailed analysis of what was modified and why
- **Multi-format Support**: Handles PDF, DOCX, and plain text resume files
- **Web Scraping**: Automatically extracts job requirements from posting URLs
- **Serverless Architecture**: Scales automatically with zero infrastructure management

## Architecture

### AWS Services Used
- **Lambda**: Go-based function for resume processing and AI generation
- **API Gateway**: RESTful API with CORS support
- **Bedrock Nova**: AI model for content generation and optimization
- **S3**: Storage for Lambda code and CloudFormation templates
- **CloudWatch**: Logging and monitoring
- **IAM**: Security and permissions management

### System Flow
1. User uploads resume file and job URL via API
2. Lambda function parses resume (PDF/DOCX/text)
3. Web scraper extracts job description from URL
4. Amazon Bedrock Nova generates optimized resume, cover letter, and change summary
5. Results returned as structured JSON response

### Project Structure
```
├── main.go              # Lambda function source code
├── go.mod               # Go dependencies
├── Makefile            # Build and deployment commands
├── deploy.sh           # Deployment automation script
├── params.json         # CloudFormation parameters
├── main.yaml           # Root CloudFormation template
├── lambda.yaml         # Lambda function and IAM resources
└── api.yaml            # API Gateway configuration
```

## API Endpoints

### POST `/api/customize-resume`
Customizes resume based on job posting.

**Request:**
```json
{
  "resume": "base64-encoded-file-content",
  "jobUrl": "https://company.com/job-posting",
  "fileName": "resume.pdf"
}
```

**Response:**
```json
{
  "resume": "optimized-resume-content",
  "coverLetter": "generated-cover-letter",
  "changes": "summary-of-modifications",
  "metadata": {
    "name": "John Doe",
    "company": "Target Company",
    "position": "Software Engineer"
  }
}
```

### GET `/health`
Health check endpoint.

## Prerequisites

- AWS CLI configured with appropriate permissions
- Go 1.21+
- Make (optional, for convenience commands)

### Required AWS Permissions
- CloudFormation stack management
- Lambda function deployment
- S3 bucket creation and management
- API Gateway configuration
- Bedrock model access (Nova Pro)
- IAM role creation

## Deployment

### Quick Start
1. **Clone and configure:**
   ```bash
   git clone <repository>
   cd resume-customizer
   export AWS_PROFILE=your-profile
   ```

2. **Deploy everything:**
   ```bash
   make deploy
   ```

3. **Get API endpoint:**
   ```bash
   make outputs
   ```

### Manual Deployment
```bash
# Initialize Go dependencies
make init

# Build Lambda function
make build

# Deploy infrastructure
./deploy.sh deploy

# Check deployment status
make status
```

### Configuration Options

Environment variables can be set in `params.json` or via command line:

```bash
# Custom deployment
AWS_PROFILE=myprofile STACK_NAME=MyResumeStack make deploy

# Deploy to different environment
make staging-deploy    # or make prod-deploy
```

### Available Make Commands
- `make help` - Show all available commands
- `make build` - Build Go Lambda function
- `make test` - Run Go tests
- `make deploy` - Full deployment
- `make update-lambda` - Update only Lambda code
- `make logs` - View Lambda logs
- `make status` - Check stack status
- `make outputs` - Show API endpoints
- `make clean` - Remove build artifacts

## Development

### Local Testing
```bash
# Run tests
go test -v ./...

# Build locally
make dev-build

# Quick code deployment
make dev-deploy
```

### Debugging
```bash
# View real-time logs
make logs

# Check stack events
aws cloudformation describe-stack-events --stack-name ResumeCustomizerStack
```

## Configuration

### Key Parameters (params.json)
- `LambdaTimeout`: Function timeout (default: 60s)
- `LambdaMemorySize`: Memory allocation (default: 512MB)
- `CorsOrigin`: CORS policy (default: "*")

### Environment Variables
Set in Lambda environment:
- `ENVIRONMENT`: Deployment environment (dev/staging/prod)

## Security

- No hardcoded credentials or API keys
- IAM roles with least privilege access
- CORS configured for web access
- S3 buckets with versioning enabled
- CloudWatch logging for audit trails

## Supported File Formats

- **PDF**: Full text extraction
- **DOCX**: Coming soon (returns helpful error)
- **Plain Text**: Direct processing

## Cost Optimization

- ARM64 Lambda for better price/performance
- Pay-per-request pricing model
- 14-day log retention
- Optimized memory allocation

## Troubleshooting

### Common Issues
1. **Deployment fails**: Check AWS credentials and permissions
2. **Lambda timeout**: Increase timeout in params.json
3. **Job scraping fails**: URL may be behind authentication or have anti-bot protection
4. **PDF parsing errors**: Ensure file is not password-protected or corrupted

### Debug Commands
```bash
# Check AWS configuration
make check-aws

# Validate templates before deployment
make validate

# View detailed logs
aws logs tail /aws/lambda/ResumeCustomizerStack-ResumeCustomizerFunction --follow
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes and test
4. Submit a pull request

## License

This project is open source. See LICENSE file for details.

## Support

For issues and questions:
- Check CloudWatch logs for error details
- Review AWS CloudFormation console for deployment issues
- Ensure Bedrock Nova access is enabled in your AWS account