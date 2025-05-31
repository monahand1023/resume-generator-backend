# Resume Customizer Service Makefile

.PHONY: help build test clean deploy update-lambda validate create-stack update-stack delete-stack

# Default AWS profile and region
AWS_PROFILE ?= monahand
AWS_REGION ?= us-west-2
STACK_NAME ?= ResumeCustomizerStack
ENVIRONMENT ?= dev

# Colors for output
GREEN := \033[0;32m
RED := \033[0;31m
YELLOW := \033[1;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

help: ## Show this help message
	@echo "$(YELLOW)Resume Customizer Service - Available Commands:$(NC)"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "$(BLUE)%-20s$(NC) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(YELLOW)Environment Variables:$(NC)"
	@echo "$(BLUE)AWS_PROFILE$(NC)     Current: $(AWS_PROFILE)"
	@echo "$(BLUE)AWS_REGION$(NC)      Current: $(AWS_REGION)"
	@echo "$(BLUE)STACK_NAME$(NC)      Current: $(STACK_NAME)"
	@echo "$(BLUE)ENVIRONMENT$(NC)     Current: $(ENVIRONMENT)"

build: ## Build the Go Lambda function
	@echo "$(YELLOW)Building Go Lambda function...$(NC)"
	./deploy.sh build

test: ## Run Go tests
	@echo "$(YELLOW)Running tests...$(NC)"
	go test -v ./...

clean: ## Clean build artifacts
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	rm -f bootstrap resume-customizer.zip
	go clean

validate: ## Validate CloudFormation templates
	@echo "$(YELLOW)Validating CloudFormation templates...$(NC)"
	./deploy.sh validate

deploy: ## Full deployment (build + upload + create/update stack)
	@echo "$(YELLOW)Starting full deployment...$(NC)"
	./deploy.sh deploy

update-lambda: ## Update only the Lambda function code
	@echo "$(YELLOW)Updating Lambda function code...$(NC)"
	./deploy.sh update-lambda

create-stack: ## Create CloudFormation stack
	@echo "$(YELLOW)Creating CloudFormation stack...$(NC)"
	./deploy.sh create

update-stack: ## Update CloudFormation stack
	@echo "$(YELLOW)Updating CloudFormation stack...$(NC)"
	./deploy.sh update

delete-stack: ## Delete CloudFormation stack
	@echo "$(RED)Deleting CloudFormation stack...$(NC)"
	./deploy.sh delete

upload-templates: ## Upload CloudFormation templates to S3
	@echo "$(YELLOW)Uploading templates...$(NC)"
	./deploy.sh upload-templates

upload-code: ## Upload Lambda code to S3
	@echo "$(YELLOW)Uploading Lambda code...$(NC)"
	./deploy.sh upload-code

logs: ## Show Lambda function logs
	@echo "$(YELLOW)Showing Lambda logs...$(NC)"
	aws logs tail /aws/lambda/$(STACK_NAME)-ResumeCustomizerFunction --follow --profile $(AWS_PROFILE)

status: ## Show stack status
	@echo "$(YELLOW)Stack Status:$(NC)"
	@aws cloudformation describe-stacks --stack-name $(STACK_NAME) --profile $(AWS_PROFILE) --region $(AWS_REGION) --query 'Stacks[0].[StackName,StackStatus,CreationTime]' --output table 2>/dev/null || echo "$(RED)Stack not found$(NC)"

outputs: ## Show stack outputs
	@echo "$(YELLOW)Stack Outputs:$(NC)"
	@aws cloudformation describe-stacks --stack-name $(STACK_NAME) --profile $(AWS_PROFILE) --region $(AWS_REGION) --query 'Stacks[0].Outputs[*].[OutputKey,OutputValue,Description]' --output table 2>/dev/null || echo "$(RED)Stack not found or no outputs$(NC)"

deps: ## Install/update Go dependencies
	@echo "$(YELLOW)Installing Go dependencies...$(NC)"
	go mod tidy
	go mod download

init: ## Initialize project (create go.mod if not exists)
	@echo "$(YELLOW)Initializing project...$(NC)"
	@if [ ! -f go.mod ]; then \
		echo "Creating go.mod..."; \
		go mod init resume-customizer; \
		echo "$(GREEN)go.mod created$(NC)"; \
	else \
		echo "$(BLUE)go.mod already exists$(NC)"; \
	fi
	@make deps

check-aws: ## Check AWS credentials and configuration
	@echo "$(YELLOW)Checking AWS configuration...$(NC)"
	@aws sts get-caller-identity --profile $(AWS_PROFILE) --region $(AWS_REGION) && echo "$(GREEN)✓ AWS credentials valid$(NC)" || echo "$(RED)✗ AWS credentials invalid$(NC)"

# Development helpers
dev-build: build ## Build for development
	@echo "$(GREEN)✓ Development build complete$(NC)"

dev-deploy: build upload-code update-lambda ## Quick development deployment (code only)
	@echo "$(GREEN)✓ Development deployment complete$(NC)"

# Production helpers
prod-deploy: ## Deploy to production
	@echo "$(YELLOW)Deploying to production...$(NC)"
	@$(MAKE) ENVIRONMENT=prod deploy

staging-deploy: ## Deploy to staging
	@echo "$(YELLOW)Deploying to staging...$(NC)"
	@$(MAKE) ENVIRONMENT=staging deploy