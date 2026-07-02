import logo from '@/assets/images/flowise_white.svg'
import logoDark from '@/assets/images/flowise_dark.svg'

import { useSelector } from 'react-redux'
import { useConfig } from '@/store/context/ConfigContext'

// ==============================|| LOGO ||============================== //

const Logo = () => {
    const customization = useSelector((state) => state.customization)
    const { config } = useConfig()
    const appName = config?.APP_NAME || 'Farindra Agentic'

    return (
        <div style={{ alignItems: 'center', display: 'flex', flexDirection: 'row', marginLeft: '10px', gap: '10px' }}>
            <img style={{ objectFit: 'contain', height: 32, width: 32 }} src={customization.isDarkMode ? logoDark : logo} alt={appName} />
            <span
                style={{
                    fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
                    fontSize: '17px',
                    fontWeight: 700,
                    letterSpacing: '0.2px',
                    color: customization.isDarkMode ? '#ffffff' : '#1e1b4b',
                    whiteSpace: 'nowrap'
                }}
            >
                {appName}
            </span>
        </div>
    )
}

export default Logo
