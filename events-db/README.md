# Events Database and API 


## github.com/ulule/limiter/v3 might not need
## go get github.com/redis/go-redis/v9

<img width="1536" height="1024" alt="image" src="https://github.com/user-attachments/assets/f0529507-d4ae-42c8-890b-5c09bb39d63a" />

Getting started & client tokens: https://docs.stitch.money/
Pay By Bank integration (your primary flow): https://docs.stitch.money/payment-products/payins/paybybank/integration-process
Card (hosted UI): https://docs.stitch.money/payment-products/payins/card/once-off/hosted-ui
Wallet payments (Apple/Google/Samsung Pay): https://docs.stitch.money/payment-products/payins/wallets/once-off/web-integration-process
Webhook setup & verification: https://docs.stitch.money/webhooks/using_webhooks
Webhook signature verification (legacy format, matches your implementation approach): https://docs.stitch.money/webhooks_legacy
Refunds: https://docs.stitch.money/payment-products/payouts/refunds
Frontend iframe integration: https://docs.stitch.money/safe-link/frontend-integration
PaymentInitiationRequest type reference: https://docs.stitch.money/api/objects/payment-initiation-request

docker run -d --name test-postgres \
>>      -e POSTGRES_USER=testuser \  
>>      -e POSTGRES_PASSWORD=testpass \  
>>      -e POSTGRES_DB=test_db \  
>>      -p 5433:5432 postgres:15 