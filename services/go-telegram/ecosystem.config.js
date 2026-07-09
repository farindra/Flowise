module.exports = {
    apps: [
        {
            name: 'go-telegram',
            script: './start.sh',
            cwd: '/www/wwwroot/agentic.oceanbearings.co.id/services/go-telegram',
            interpreter: 'bash',
            out_file: '/www/wwwroot/agentic.oceanbearings.co.id/packages/server/logs/go-telegram-out.log',
            error_file: '/www/wwwroot/agentic.oceanbearings.co.id/packages/server/logs/go-telegram-error.log',
            merge_logs: true
        }
    ]
}
