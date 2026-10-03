# GitHub token for private repositories

stackploy clones private GitHub repositories with the token in the `GITHUB_TOKEN` environment variable. It uses the token only for `https://github.com/...` repository URLs. Use a fine-grained personal access token that can only read the repositories you deploy.

## Create the token

1. On GitHub, go to **Settings → Developer settings → Personal access tokens → Fine-grained tokens**.
2. Select **Generate new token**.
3. Set **Token name** to `stackploy`.
4. Set **Expiration**. Choose the shortest period you are willing to rotate on, such as 90 days.
5. Set **Resource owner** to the account or organization that owns the repositories.
6. Under **Repository access**, select **Only select repositories**. Then select only the repositories that stackploy deploys.
7. Under **Permissions → Repository permissions**, set **Contents** to **Read-only**.
   GitHub adds **Metadata: Read-only** automatically. That permission is required and cannot be removed.
8. Leave all other permissions at **No access**.
9. Select **Generate token** and copy the token.

If the resource owner is an organization, an organization owner may need to approve the token before it works.

## Give the token to stackploy

Set `GITHUB_TOKEN` on the stackploy container. For example, in `compose.yaml`:

```yaml
services:
  stackploy:
    environment:
      GITHUB_TOKEN: ${GITHUB_TOKEN}
```

Then restart the container.

stackploy passes the token to `git clone` as an HTTP header through git's environment config. The token is not written to the clone's `.git/config` or to the database.

## Rotate the token

Before the token expires, generate a new token with the same settings. Update `GITHUB_TOKEN`, restart stackploy, and delete the old token on GitHub.
