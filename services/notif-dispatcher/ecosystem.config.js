module.exports = {
    apps: [
        {
            name: 'notif-dispatcher',
            script: 'index.js',
            cwd: '/www/wwwroot/agentic.oceanbearings.co.id/services/notif-dispatcher',
            env: {
                WA_SESSION_ID: 'e6a7c815-7316-4590-9f70-a15f7c7e4def',
                CRM_INTERNAL_KEY: 'ob-crm-internal-2026',
                WA_INTERNAL_KEY: 'ob-wa-internal-2026',
                INTERVAL_MS: '300000'
            }
        }
    ]
}
