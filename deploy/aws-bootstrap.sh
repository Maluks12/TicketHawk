#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# One-time AWS setup for the build-off-box pipeline.
#
# Run this ONCE from a machine with admin AWS credentials (your laptop, or the
# EC2 box if its instance profile is permissive enough). It is idempotent —
# re-running it is safe and will just report that things already exist.
#
# What it creates:
#   1. Three ECR repositories, with a lifecycle policy so old images don't
#      accumulate charges.
#   2. A GitHub OIDC identity provider, so Actions can assume a role without
#      any long-lived AWS access keys stored in GitHub.
#   3. An IAM role that can only push to those three repositories.
#
# Usage:
#   chmod +x deploy/aws-bootstrap.sh
#   GITHUB_REPO=Maluks12/EventsProject ./deploy/aws-bootstrap.sh
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

REGION="${AWS_REGION:-af-south-1}"
GITHUB_REPO="${GITHUB_REPO:-}"
ROLE_NAME="tickethawk-github-deploy"
REPOS=(tickethawk-api tickethawk-admin tickethawk-organiser)

if [[ -z "$GITHUB_REPO" ]]; then
  echo "Set GITHUB_REPO, e.g. GITHUB_REPO=Maluks12/EventsProject $0" >&2
  exit 1
fi

ACCOUNT_ID="$(aws sts get-caller-identity --query Account --output text)"
REGISTRY="${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com"

echo "Account : $ACCOUNT_ID"
echo "Region  : $REGION"
echo "Repo    : $GITHUB_REPO"
echo

# ── 1. ECR repositories ─────────────────────────────────────────────────────
for repo in "${REPOS[@]}"; do
  if aws ecr describe-repositories --repository-names "$repo" --region "$REGION" >/dev/null 2>&1; then
    echo "ECR  $repo — exists"
  else
    aws ecr create-repository \
      --repository-name "$repo" \
      --region "$REGION" \
      --image-scanning-configuration scanOnPush=true \
      --image-tag-mutability MUTABLE >/dev/null
    echo "ECR  $repo — created"
  fi

  # Untagged layers pile up on every rebuild and are billed per GB. Keep the
  # last 10 tagged images for rollback, bin untagged ones after a day.
  aws ecr put-lifecycle-policy \
    --repository-name "$repo" \
    --region "$REGION" \
    --lifecycle-policy-text '{
      "rules": [
        {
          "rulePriority": 1,
          "description": "Expire untagged images after 1 day",
          "selection": {
            "tagStatus": "untagged",
            "countType": "sinceImagePushed",
            "countUnit": "days",
            "countNumber": 1
          },
          "action": { "type": "expire" }
        },
        {
          "rulePriority": 2,
          "description": "Keep only the 10 most recent tagged images",
          "selection": {
            "tagStatus": "any",
            "countType": "imageCountMoreThan",
            "countNumber": 10
          },
          "action": { "type": "expire" }
        }
      ]
    }' >/dev/null
done
echo

# ── 2. GitHub OIDC provider ─────────────────────────────────────────────────
OIDC_ARN="arn:aws:iam::${ACCOUNT_ID}:oidc-provider/token.actions.githubusercontent.com"
if aws iam get-open-id-connect-provider --open-id-connect-provider-arn "$OIDC_ARN" >/dev/null 2>&1; then
  echo "OIDC provider — exists"
else
  aws iam create-open-id-connect-provider \
    --url "https://token.actions.githubusercontent.com" \
    --client-id-list "sts.amazonaws.com" \
    --thumbprint-list "6938fd4d98bab03faadb97b34396831e3780aea1" >/dev/null
  echo "OIDC provider — created"
fi
echo

# ── 3. Deploy role ──────────────────────────────────────────────────────────
# The trust policy is scoped to THIS repository. Without the sub condition any
# GitHub repo in the world could assume this role.
TRUST=$(cat <<JSON
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Federated": "${OIDC_ARN}" },
    "Action": "sts:AssumeRoleWithWebIdentity",
    "Condition": {
      "StringEquals": { "token.actions.githubusercontent.com:aud": "sts.amazonaws.com" },
      "StringLike":   { "token.actions.githubusercontent.com:sub": "repo:${GITHUB_REPO}:*" }
    }
  }]
}
JSON
)

RESOURCES=$(printf '"arn:aws:ecr:%s:%s:repository/%s",' "$REGION" "$ACCOUNT_ID" "${REPOS[@]}" | sed 's/,$//')
POLICY=$(cat <<JSON
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "ecr:GetAuthorizationToken",
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "ecr:BatchCheckLayerAvailability",
        "ecr:CompleteLayerUpload",
        "ecr:InitiateLayerUpload",
        "ecr:PutImage",
        "ecr:UploadLayerPart",
        "ecr:BatchGetImage",
        "ecr:GetDownloadUrlForLayer"
      ],
      "Resource": [${RESOURCES}]
    }
  ]
}
JSON
)

if aws iam get-role --role-name "$ROLE_NAME" >/dev/null 2>&1; then
  aws iam update-assume-role-policy --role-name "$ROLE_NAME" --policy-document "$TRUST" >/dev/null
  echo "IAM role $ROLE_NAME — trust policy updated"
else
  aws iam create-role --role-name "$ROLE_NAME" --assume-role-policy-document "$TRUST" >/dev/null
  echo "IAM role $ROLE_NAME — created"
fi

aws iam put-role-policy \
  --role-name "$ROLE_NAME" \
  --policy-name "ecr-push" \
  --policy-document "$POLICY" >/dev/null
echo "IAM role $ROLE_NAME — push policy attached"
echo

cat <<DONE
─────────────────────────────────────────────────────────────────────────────
Done. Two things to set now.

1. In GitHub → Settings → Secrets and variables → Actions:

   Secret   AWS_DEPLOY_ROLE_ARN
            arn:aws:iam::${ACCOUNT_ID}:role/${ROLE_NAME}

   Variable NEXT_PUBLIC_GOOGLE_MAPS_KEY
            (your browser Maps key — public by design, but a variable keeps it
             out of the workflow file)

2. In events-db/.env.production on the EC2 box, add:

   ECR_REGISTRY=${REGISTRY}
   IMAGE_TAG=latest

Then push to main and watch the Actions tab.
─────────────────────────────────────────────────────────────────────────────
DONE
