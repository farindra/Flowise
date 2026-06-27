module.exports = {
    apps: [{
        name: 'go-wa',
        script: './bin/go-wa',
        cwd: '/www/wwwroot/agentic.oceanbearings.co.id/services/go-wa',
        env: {
            DB_HOST: '127.0.0.1',
            DB_PORT: '5432',
            DB_USER: 'flowise',
            DB_PASSWORD: 'xfydBwYFRTSZT6ia',
            DB_NAME: 'flowise',
            FLOWISE_BASE_URL: 'https://agentic.oceanbearings.co.id',
            FLOWISE_API_KEY: 'gpMebq4hbHBJPIKE_nk13m3CAn7h4nAyrntyTmLuzZE',
            DATA_DIR: '/data/wa-sessions',
            INTERNAL_API_KEY: 'ob-wa-internal-2026',
            PORT: '8082',
            FLOWISE_TIMEOUT: '120'
        }
    }]
}
