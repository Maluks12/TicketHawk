#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Deploy on EC2: authenticate to ECR, pull CI-built images, restart.
#
# This replaces `docker compose up -d --build`, which compiled three images on
# the production box and degraded the live API for ~20 minutes each time. Here
# nothing is compiled — it downloads layers and swaps containers, so expect
# tens of seconds.
#
# Usage:
#   chmod +x deploy/deploy.sh
#   ./deploy/deploy.sh                # latest
#   IMAGE_TAG=<git-sha> ./deploy/deploy.sh   # pin / roll back
#
# Requires in events-db/.env.production:
#   ECR_REGISTRY=<acct>.dkr.ecr.af-south-1.amazonaws.com
#   IMAGE_TAG=latest
#
# And an instance profile that can read ECR. Minimum policy:
#   ecr:GetAuthorizationToken, ecr:BatchGetImage,
#   ecr:GetDownloadUrlForLayer, ecr:BatchCheckLayerAvailability
# (AWS managed AmazonEC2ContainerRegistryReadOnly covers all four.)
#
# TO AUTOMATE LATER: the security group blocks all inbound, so CI cannot SSH in.
# Use SSM instead — attach AmazonSSMManagedInstanceCore to the instance profile,
# then add a workflow step running:
#   aws ssm send-command --instance-ids <id> \
#     --document-name AWS-RunShellScript \
#     --parameters 'commands=["cd /home/ubuntu/EventsProject && ./deploy/deploy.sh"]'
# No inbound ports, no SSH keys in GitHub.
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

REGION="${AWS_REGION:-af-south-1}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_DIR="${REPO_ROOT}/events-db"
COMPOSE_FILE="docker-compose.prod.yml"

cd "$COMPOSE_DIR"

if [[ ! -f .env.production ]]; then
  echo "No .env.production in ${COMPOSE_DIR}" >&2
  exit 1
fi

# Read ECR_REGISTRY without sourcing the whole file — .env.production holds
# secrets and unquoted values that a naive `source` would mangle or leak.
ECR_REGISTRY="$(grep -E '^ECR_REGISTRY=' .env.production | tail -1 | cut -d= -f2- | tr -d '"'"'"' ')"
if [[ -z "${ECR_REGISTRY}" ]]; then
  echo "ECR_REGISTRY not set in .env.production — run deploy/aws-bootstrap.sh first" >&2
  exit 1
fi

echo "── Pulling code ────────────────────────────────────────────────"
git -C "$REPO_ROOT" pull --ff-only

echo
echo "── Authenticating to ECR ───────────────────────────────────────"
# The token lasts 12 hours; re-running is cheap and avoids stale-auth failures.
aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "$ECR_REGISTRY"

echo
echo "── Pulling images ──────────────────────────────────────────────"
docker compose -f "$COMPOSE_FILE" pull api admin organiser

echo
echo "── Restarting ──────────────────────────────────────────────────"
# No --build. Migrations still run from the api container's entrypoint, exactly
# as before.
docker compose -f "$COMPOSE_FILE" up -d

echo
echo "── Reclaiming space ────────────────────────────────────────────"
# Superseded images accumulate fast with per-commit tags, and a full disk
# silently breaks Docker. Images only, never volumes — redis-data lives there.
docker image prune -f

echo
docker compose -f "$COMPOSE_FILE" ps
echo
echo "Done. Logs:  docker compose -f ${COMPOSE_FILE} logs -f --tail 50"
