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
            max_memory_restart: '4G',
            log_date_format: 'YYYY-MM-DD HH:mm:ss',
            error_file: '/www/wwwroot/agentic.oceanbearings.co.id/logs/flowise-error.log',
            out_file: '/www/wwwroot/agentic.oceanbearings.co.id/logs/flowise-out.log',
            merge_logs: true,
            autorestart: true,
            watch: false,
            restart_delay: 5000
        }
    ]
}
