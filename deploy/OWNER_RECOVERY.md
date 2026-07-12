# Owner bootstrap and recovery runbook

This runbook is for operators who control the instance host and its Docker daemon. Treat that access as equivalent to database-root access. Never expose `pmsctl` through HTTP or delegate Docker access only to run this procedure.

## Bootstrap a new instance

After the deployment reports healthy, create the first owner from the active backend container. Read the password without echoing it or storing it in shell history:

```sh
cd /opt/progo/app
read -r -s OWNER_PASSWORD
printf '%s\n' "$OWNER_PASSWORD" | ./deploy/pmsctl.sh owner create \
  --username owner \
  --email owner@example.test \
  --password-stdin
unset OWNER_PASSWORD
```

Sign in immediately. The application redirects a new owner to the account page until MFA enrollment is complete. Store the one-time recovery codes in an approved offline password vault, then enroll a second trusted owner with MFA. The database rejects demotion of the final owner, including concurrent demotion attempts.

## Recovery order

Use the least-privileged path that still works:

1. Use the normal password-reset email when the owner controls their verified mailbox.
2. Use a single-use MFA recovery code when only the authenticator is unavailable.
3. Use another active owner to contain a compromised account and preserve administrative access.
4. Use offline recovery only when the supported browser recovery paths are unavailable.

## Offline credential recovery

Before recovery, use a trusted SSH session, confirm the instance name and target username out of band, and create a verified backup:

```sh
cd /opt/progo/app
./deploy/backup.sh
```

Reset the password while retaining the existing MFA factor:

```sh
read -r -s OWNER_PASSWORD
printf '%s\n' "$OWNER_PASSWORD" | ./deploy/pmsctl.sh owner recover \
  --username owner \
  --confirm-username owner \
  --password-stdin
unset OWNER_PASSWORD
```

Add `--reset-mfa` only when the owner has lost both the authenticator and every recovery code. Offline recovery:

- works only for an existing `owner` account;
- requires the username to be repeated exactly;
- increments the account token version;
- revokes all browser sessions and pending email challenges;
- optionally removes the stored MFA credential; and
- records an `owner.recovered` security event with `pmsctl` as the client.

After recovery, sign in with the replacement password, enroll MFA again when it was reset, store the new recovery codes offline, review the account security history, and verify that at least two trusted owners remain. If host or Docker access may have been compromised, also rotate database, JWT, SMTP, OAuth, metrics, and actor-key encryption secrets using the normal secret-rotation procedure.
