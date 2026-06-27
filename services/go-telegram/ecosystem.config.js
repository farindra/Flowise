module.exports = {
    apps: [{
        name: 'go-telegram',
        script: './bin/go-telegram',
        cwd: '/www/wwwroot/agentic.oceanbearings.co.id/services/go-telegram',
        env: {
            DB_HOST: '127.0.0.1',
            DB_PORT: '5432',
            DB_USER: 'flowise',
            DB_PASSWORD: 'xfydBwYFRTSZT6ia',
            DB_NAME: 'flowise',
            FLOWISE_BASE_URL: 'https://agentic.oceanbearings.co.id',
            FLOWISE_API_KEY: 'gpMebq4hbHBJPIKE_nk13m3CAn7h4nAyrntyTmLuzZE',
            FLOWISE_CHATFLOW_CUSTOMER: '75354d20-6b60-44bb-b473-9e4a7ad96288',
            FLOWISE_CHATFLOW_OWNER: 'cc5dbfcc-6767-4914-852d-8aa46dfc3934',
            SALESMAN_BOT_TOKEN: '8927501389:AAGk0YbBZIbeHOIFVw2tH-lmkCt-34txgTg',
            FLOWISE_CHATFLOW_SALESMAN: '17007b30-34d8-43c9-a03d-15bb86378f79',
            TELEGRAM_BOT_TOKEN: '8662097755:AAEFh66yKYn9cV_P2jQeaMgV2liKji6RsUI',
            OWNER_BOT_TOKEN: '8750761601:AAFMijCN9_9dCH-rb_Na3Dz-tGIW0PSxoSM',
            OWNER_TELEGRAM_IDS: '1486676978,368257367',
            HUMAN_TELEGRAM: '@ob_admin',
            INTERNAL_API_KEY: 'ob-tg-internal-2026',
            WEBHOOK_BASE_URL: 'https://agentic.oceanbearings.co.id/telegram',
            PORT: '8081',
            FLOWISE_TIMEOUT: '120',
            WAIT_MSG_INTERVAL: '6'
        }
    }]
}
