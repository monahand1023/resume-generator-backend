#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration variables (can be overridden with command line arguments)
AWS_PROFILE=${AWS_PROFILE:-"monahand"}
S3_BUCKET=${S3_BUCKET:-"resume-customizer-cf-templates"}
LAMBDA_CODE_BUCKET=${LAMBDA_CODE_BUCKET:-"resume-customizer-lambda-code"}
STACK_NAME=${STACK_NAME:-"ResumeCustomizerStack"}
PARAM_FILE=${PARAM_FILE:-"params.json"}
REGION=${REGION:-"us-west-2"}

# Command to execute
COMMAND=""

# Templates to upload
TEMPLATES=(
    "main.yaml"
    "lambda.yaml"
    "api.yaml"
)

# Display usage information
usage() {
    echo -e "${YELLOW}Resume Customizer CloudFormation Management Script${NC}"
    echo ""
    echo "Usage: $0 [OPTIONS] COMMAND"
    echo ""
    echo "Commands:"
    echo "  build                 Build Go Lambda function"
    echo "  upload-code           Upload Lambda code to S3"
    echo "  upload-templates      Upload CloudFormation templates to S3"
    echo "  update-lambda         Update Lambda function code only"
    echo "  create                Create CloudFormation stack"
    echo "  update                Update CloudFormation stack"
    echo "  delete                Delete CloudFormation stack"
    echo "  validate              Validate CloudFormation templates"
    echo "  deploy                Full deployment (build + upload + update)"
    echo ""
    echo "Options:"
    echo "  -p, --profile PROFILE       AWS profile to use (default: monahand)"
    echo "  -b, --bucket BUCKET         S3 bucket for CF templates (default: resume-customizer-cf-templates)"
    echo "  -l, --lambda-bucket BUCKET  S3 bucket for Lambda code (default: resume-customizer-lambda-code)"
    echo "  -s, --stack STACK           Stack name (default: ResumeCustomizerStack)"
    echo "  -f, --param-file FILE       Parameter file (default: params.json)"
    echo "  -r, --region REGION         AWS region (default: us-west-2)"
    echo "  -c, --changeset NAME        Changeset name for updates (default: auto-generated)"
    echo "  -h, --help                  Display this help message"
    echo ""
}

# Parse command line options
parse_options() {
    local args=()

    while [[ $# -gt 0 ]]; do
        case "$1" in
            build|upload-code|upload-templates|update-lambda|create|update|delete|validate|deploy)
                COMMAND="$1"
                shift
                ;;
            -p|--profile)
                AWS_PROFILE="$2"
                shift 2
                ;;
            -b|--bucket)
                S3_BUCKET="$2"
                shift 2
                ;;
            -l|--lambda-bucket)
                LAMBDA_CODE_BUCKET="$2"
                shift 2
                ;;
            -s|--stack)
                STACK_NAME="$2"
                shift 2
                ;;
            -f|--param-file)
                PARAM_FILE="$2"
                shift 2
                ;;
            -r|--region)
                REGION="$2"
                shift 2
                ;;
            -c|--changeset)
                CHANGE_SET_NAME="$2"
                shift 2
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                args+=("$1")
                shift
                ;;
        esac
    done

    # If no command was specified and there are positional args, use the first as command
    if [[ -z "$COMMAND" && ${#args[@]} -gt 0 ]]; then
        COMMAND="${args[0]}"
    fi

    # Validate command
    if [[ -z "$COMMAND" ]]; then
        echo -e "${RED}Error: No command specified${NC}"
        usage
        exit 1
    fi

    # Generate default changeset name if not provided for update command
    if [[ "$COMMAND" == "update" && -z "$CHANGE_SET_NAME" ]]; then
        CHANGE_SET_NAME="update-$(date +%Y%m%d-%H%M%S)"
    fi
}

# Configure AWS profile
configure_aws() {
    echo -e "${BLUE}Configuring AWS with profile: ${AWS_PROFILE}${NC}"
    export AWS_PROFILE=$AWS_PROFILE
    export AWS_REGION=$REGION

    # Verify AWS CLI is working
    if ! aws sts get-caller-identity &>/dev/null; then
        echo -e "${RED}Error: Failed to authenticate with AWS. Check your credentials and profile.${NC}"
        exit 1
    fi

    # Get account ID for logging
    ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
    echo -e "${GREEN}✓ Authenticated with AWS account: ${ACCOUNT_ID}${NC}"

    # Update bucket names with account ID for uniqueness
    LAMBDA_CODE_BUCKET="${LAMBDA_CODE_BUCKET}-${ACCOUNT_ID}"
}

# Check if S3 bucket exists, create if not
ensure_s3_bucket() {
    local bucket_name=$1
    local bucket_type=$2

    if ! aws s3api head-bucket --bucket $bucket_name 2>/dev/null; then
        echo -e "${YELLOW}S3 bucket '$bucket_name' does not exist. Creating...${NC}"
        aws s3 mb s3://$bucket_name --region $REGION

        # Enable versioning on the bucket
        aws s3api put-bucket-versioning \
            --bucket $bucket_name \
            --versioning-configuration Status=Enabled

        echo -e "${GREEN}✓ Created S3 bucket '$bucket_name' with versioning enabled${NC}"
    else
        echo -e "${GREEN}✓ S3 bucket '$bucket_name' exists${NC}"
    fi
}

# Build Go Lambda function
build_lambda() {
    echo -e "${YELLOW}Building Go Lambda function...${NC}"

    # Check if go.mod exists
    if [[ ! -f "go.mod" ]]; then
        echo -e "${RED}Error: go.mod not found. Make sure you're in the project root directory.${NC}"
        exit 1
    fi

    # Clean up any previous builds
    rm -f bootstrap resume-customizer.zip

    echo -e "${BLUE}Running go mod tidy...${NC}"
    go mod tidy

    echo -e "${BLUE}Building Lambda binary for Linux ARM64...${NC}"
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap main.go

    echo -e "${BLUE}Creating deployment package...${NC}"
    zip resume-customizer.zip bootstrap

    echo -e "${GREEN}✓ Lambda function built successfully${NC}"
}

# Upload Lambda code to S3
upload_lambda_code() {
    echo -e "${YELLOW}Uploading Lambda code to S3...${NC}"

    ensure_s3_bucket $LAMBDA_CODE_BUCKET "Lambda code"

    # Check if deployment package exists
    if [[ ! -f "resume-customizer.zip" ]]; then
        echo -e "${YELLOW}Deployment package not found. Building first...${NC}"
        build_lambda
    fi

    echo -e "${BLUE}Uploading resume-customizer.zip to S3...${NC}"
    aws s3 cp resume-customizer.zip s3://${LAMBDA_CODE_BUCKET}/

    echo -e "${GREEN}✓ Lambda code uploaded successfully${NC}"
}

# Upload CloudFormation templates to S3
upload_templates() {
    echo -e "${YELLOW}Uploading CloudFormation templates to S3 bucket: $S3_BUCKET${NC}"
    ensure_s3_bucket $S3_BUCKET "CloudFormation templates"

    for template in "${TEMPLATES[@]}"; do
        if [[ ! -f "$template" ]]; then
            echo -e "${RED}Warning: Template file '$template' not found, skipping${NC}"
            continue
        fi

        echo -e "${BLUE}Uploading ${template}...${NC}"
        aws s3 cp "$template" "s3://$S3_BUCKET/$template"
    done

    echo -e "${GREEN}✓ All templates uploaded successfully${NC}"
}

# Update Lambda function code only
update_lambda_code() {
    echo -e "${YELLOW}Updating Lambda function code...${NC}"

    # Build and upload code
    build_lambda
    upload_lambda_code

    # Update the Lambda function
    echo -e "${BLUE}Updating Lambda function code...${NC}"
    aws lambda update-function-code \
        --function-name "${STACK_NAME}-ResumeCustomizerFunction" \
        --s3-bucket ${LAMBDA_CODE_BUCKET} \
        --s3-key resume-customizer.zip

    echo -e "${GREEN}✓ Lambda function updated successfully${NC}"

    # Clean up build artifacts
    rm -f bootstrap resume-customizer.zip
}

# Validate CloudFormation templates
validate_templates() {
    local has_errors=false

    echo -e "${YELLOW}Validating CloudFormation templates...${NC}"

    for template in "${TEMPLATES[@]}"; do
        if [[ ! -f "$template" ]]; then
            echo -e "${RED}Warning: Template file '$template' not found, skipping${NC}"
            continue
        fi

        echo -e "${BLUE}Validating ${template}...${NC}"
        if output=$(aws cloudformation validate-template --template-body file://$template 2>&1); then
            echo -e "${GREEN}✓ $template is valid${NC}"
        else
            echo -e "${RED}✗ $template validation failed:${NC}"
            echo "$output"
            has_errors=true
        fi
    done

    if [[ "$has_errors" == "true" ]]; then
        echo -e "${RED}✗ Some templates failed validation${NC}"
        return 1
    else
        echo -e "${GREEN}✓ All templates validated successfully${NC}"
        return 0
    fi
}

# Create CloudFormation stack
create_stack() {
    echo -e "${YELLOW}Creating CloudFormation stack: $STACK_NAME${NC}"

    # Check if stack already exists
    if aws cloudformation describe-stacks --stack-name $STACK_NAME &>/dev/null; then
        echo -e "${RED}Stack '$STACK_NAME' already exists. Use 'update' command instead.${NC}"
        exit 1
    fi

    # Build and upload Lambda code
    build_lambda
    upload_lambda_code

    # Upload templates
    upload_templates

    # Create the stack
    echo -e "${BLUE}Creating stack with main template: main.yaml${NC}"
    aws cloudformation create-stack \
        --stack-name $STACK_NAME \
        --template-url https://$S3_BUCKET.s3.$REGION.amazonaws.com/main.yaml \
        --parameters file://$PARAM_FILE \
        --capabilities CAPABILITY_IAM CAPABILITY_NAMED_IAM \
        --disable-rollback

    echo -e "${GREEN}Stack creation initiated. Waiting for stack to complete...${NC}"

    # Wait for stack creation to complete
    if aws cloudformation wait stack-create-complete --stack-name $STACK_NAME; then
        echo -e "${GREEN}✓ Stack created successfully${NC}"
        show_stack_outputs
    else
        echo -e "${RED}✗ Stack creation failed or timed out${NC}"
        echo -e "${YELLOW}Check the AWS CloudFormation console for detailed errors${NC}"
    fi

    # Clean up build artifacts
    rm -f bootstrap resume-customizer.zip
}

# Update CloudFormation stack
update_stack() {
    echo -e "${YELLOW}Updating CloudFormation stack: $STACK_NAME${NC}"

    # Check if stack exists
    if ! aws cloudformation describe-stacks --stack-name $STACK_NAME &>/dev/null; then
        echo -e "${RED}Stack '$STACK_NAME' does not exist. Use 'create' command instead.${NC}"
        exit 1
    fi

    # Build and upload Lambda code
    build_lambda
    upload_lambda_code

    # Upload templates
    upload_templates

    echo -e "${BLUE}Creating change set: $CHANGE_SET_NAME${NC}"

    # Create change set
    aws cloudformation create-change-set \
        --stack-name $STACK_NAME \
        --change-set-name $CHANGE_SET_NAME \
        --template-url https://$S3_BUCKET.s3.$REGION.amazonaws.com/main.yaml \
        --parameters file://$PARAM_FILE \
        --capabilities CAPABILITY_IAM CAPABILITY_NAMED_IAM

    # Wait for change set creation to complete
    echo -e "${BLUE}Waiting for change set creation to complete...${NC}"
    aws cloudformation wait change-set-create-complete \
        --stack-name $STACK_NAME \
        --change-set-name $CHANGE_SET_NAME

    # Describe the change set
    echo -e "${YELLOW}Changes to be applied:${NC}"
    if ! aws cloudformation describe-change-set \
        --stack-name $STACK_NAME \
        --change-set-name $CHANGE_SET_NAME; then

        echo -e "${RED}Change set creation failed or contains no changes${NC}"
        aws cloudformation describe-change-set \
            --stack-name $STACK_NAME \
            --change-set-name $CHANGE_SET_NAME \
            --query 'StatusReason' \
            --output text

        # Delete the change set and exit if there are no changes
        aws cloudformation delete-change-set \
            --stack-name $STACK_NAME \
            --change-set-name $CHANGE_SET_NAME

        # Clean up build artifacts
        rm -f bootstrap resume-customizer.zip
        exit 1
    fi

    # Auto-execute change set (remove this if you want manual confirmation)
    confirm="y"

    if [[ $confirm =~ ^[Yy]$ ]]; then
        echo -e "${BLUE}Executing change set...${NC}"
        aws cloudformation execute-change-set \
            --stack-name $STACK_NAME \
            --change-set-name $CHANGE_SET_NAME

        echo -e "${GREEN}Change set execution initiated. Waiting for update to complete...${NC}"

        # Wait for stack update to complete
        if aws cloudformation wait stack-update-complete --stack-name $STACK_NAME; then
            echo -e "${GREEN}✓ Stack updated successfully${NC}"
            show_stack_outputs
        else
            echo -e "${RED}✗ Stack update failed or timed out${NC}"
            echo -e "${YELLOW}Check the AWS CloudFormation console for detailed errors${NC}"
        fi
    else
        echo -e "${YELLOW}Change set execution cancelled${NC}"
        aws cloudformation delete-change-set \
            --stack-name $STACK_NAME \
            --change-set-name $CHANGE_SET_NAME
        echo -e "${GREEN}Change set deleted${NC}"
    fi

    # Clean up build artifacts
    rm -f bootstrap resume-customizer.zip
}

# Delete CloudFormation stack
delete_stack() {
    echo -e "${YELLOW}Deleting CloudFormation stack: $STACK_NAME${NC}"

    # Check if stack exists
    if ! aws cloudformation describe-stacks --stack-name $STACK_NAME &>/dev/null; then
        echo -e "${RED}Stack '$STACK_NAME' does not exist.${NC}"
        exit 1
    fi

    # Ask for confirmation before deleting
    echo -e "${RED}WARNING: This will delete all resources in the stack. This action cannot be undone.${NC}"
    echo -e "${YELLOW}Are you sure you want to delete the stack? (y/n)${NC}"
    read -r confirm

    if [[ $confirm =~ ^[Yy]$ ]]; then
        echo -e "${BLUE}Deleting stack...${NC}"
        aws cloudformation delete-stack --stack-name $STACK_NAME

        echo -e "${GREEN}Stack deletion initiated. Waiting for deletion to complete...${NC}"

        # Wait for stack deletion to complete
        if aws cloudformation wait stack-delete-complete --stack-name $STACK_NAME; then
            echo -e "${GREEN}✓ Stack deleted successfully${NC}"
        else
            echo -e "${RED}✗ Stack deletion failed or timed out${NC}"
            echo -e "${YELLOW}Check the AWS CloudFormation console for detailed errors${NC}"
        fi
    else
        echo -e "${YELLOW}Stack deletion cancelled${NC}"
    fi
}

# Show stack outputs
show_stack_outputs() {
    echo -e "${YELLOW}Stack Outputs:${NC}"
    aws cloudformation describe-stacks \
        --stack-name $STACK_NAME \
        --query 'Stacks[0].Outputs[*].[OutputKey,OutputValue,Description]' \
        --output table
}

# Full deployment (build + upload + update/create)
deploy() {
    echo -e "${YELLOW}Starting full deployment...${NC}"

    # Check if stack exists
    if aws cloudformation describe-stacks --stack-name $STACK_NAME &>/dev/null; then
        echo -e "${BLUE}Stack exists, performing update...${NC}"
        update_stack
    else
        echo -e "${BLUE}Stack does not exist, creating new stack...${NC}"
        create_stack
    fi
}

# Main function
main() {
    parse_options "$@"
    configure_aws

    case "$COMMAND" in
        build)
            build_lambda
            ;;
        upload-code)
            upload_lambda_code
            ;;
        upload-templates)
            upload_templates
            ;;
        update-lambda)
            update_lambda_code
            ;;
        validate)
            validate_templates
            ;;
        create)
            create_stack
            ;;
        update)
            update_stack
            ;;
        delete)
            delete_stack
            ;;
        deploy)
            deploy
            ;;
        *)
            echo -e "${RED}Error: Unknown command '$COMMAND'${NC}"
            usage
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"