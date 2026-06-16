module.exports = {
    apps: [
        {
            name: 'flowise',
            script: '/www/wwwroot/agentic.oceanbearings.co.id/packages/server/bin/run',
            args: 'start',
            cwd: '/www/wwwroot/agentic.oceanbearings.co.id/packages/server',
            interpreter: '/www/server/nodejs/v24.16.0/bin/node',
            env: {
                PATH: '/usr/local/bin:/www/server/nodejs/v24.16.0/bin:' + process.env.PATH,
                NODE_OPTIONS: '--max-old-space-size=4096'
            },
            max_memory_restart: '2G',
            log_date_format: 'YYYY-MM-DD HH:mm:ss',
            error_file: '/www/wwwroot/agentic.oceanbearings.co.id/logs/flowise-error.log',
            out_file: '/www/wwwroot/agentic.oceanbearings.co.id/logs/flowise-out.log',
            merge_logs: true,
            autorestart: true,
            watch: false,
            restart_delay: 5000
        }
        ,{
            name: 'go-telegram',
            script: '/www/wwwroot/agentic.oceanbearings.co.id/services/go-telegram/bin/go-telegram',
            cwd: '/www/wwwroot/agentic.oceanbearings.co.id/services/go-telegram',
            interpreter: 'none',
            env: {
                TELEGRAM_BOT_TOKEN: '8662097755:AAEFh66yKYn9cV_P2jQeaMgV2liKji6RsUI',
                FLOWISE_ENDPOINT: 'https://agentic.oceanbearings.co.id/api/v1/prediction/75354d20-6b60-44bb-b473-9e4a7ad96288',
                PORT: '8081',
                WEBHOOK_SECRET: 'rahasia',
                FLOWISE_TIMEOUT: '60',
                WAIT_MSG_INTERVAL: '6'
            },
            log_date_format: 'YYYY-MM-DD HH:mm:ss',
            error_file: '/www/wwwroot/agentic.oceanbearings.co.id/logs/go-telegram-error.log',
            out_file: '/www/wwwroot/agentic.oceanbearings.co.id/logs/go-telegram-out.log',
            merge_logs: true,
            autorestart: true,
            watch: false,
            restart_delay: 3000
        }
    ]
}
