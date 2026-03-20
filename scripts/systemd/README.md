# mdhub systemd deployment

These unit files assume your project is deployed at `/opt/mdhub`.

## 1. Build artifacts

```bash
cd /opt/mdhub/app/go-api/cmd/api
go build -o ../../bin/mdhub-api .

cd /opt/mdhub/app/nextjs-admin
npm ci
npm run build
```

## 2. Prepare env files

```bash
cp /opt/mdhub/app/go-api/.env.example /opt/mdhub/app/go-api/.env
cp /opt/mdhub/app/nextjs-admin/.env.example /opt/mdhub/app/nextjs-admin/.env
```

Set at least:
- `app/go-api/.env`: `MDHUB_API_TOKEN`, `MDHUB_API_PORT`
- `app/nextjs-admin/.env`: `MDHUB_API_BASE_URL`, `MDHUB_API_TOKEN`, `PORT`

## 3. Install services

```bash
sudo cp /opt/mdhub/scripts/systemd/mdhub-api.service /etc/systemd/system/
sudo cp /opt/mdhub/scripts/systemd/mdhub-admin.service /etc/systemd/system/
sudo chmod +x /opt/mdhub/scripts/start-api-prod.sh /opt/mdhub/scripts/start-admin.sh
sudo systemctl daemon-reload
sudo systemctl enable --now mdhub-api mdhub-admin
```

## 4. Check status and logs

```bash
systemctl status mdhub-api --no-pager
systemctl status mdhub-admin --no-pager
journalctl -u mdhub-api -f
journalctl -u mdhub-admin -f
```

## 5. If project path is not `/opt/mdhub`

Edit both unit files and replace:
- `WorkingDirectory=/opt/mdhub`
- `ExecStart=/opt/mdhub/...`
