import { createTheme } from '@mui/material/styles';

const theme = createTheme({
  palette: {
    mode: 'light',
    primary: {
      light: '#4a9a7a',
      main: '#0f6b4e',
      dark: '#0a4e39',
      contrastText: '#fff'
    },
    secondary: {
      light: '#d88455',
      main: '#bd6435',
      dark: '#8f421e',
      contrastText: '#fff'
    },
    background: {
      default: '#f4efe6',
      paper: 'rgba(255, 255, 255, 0.84)'
    },
    text: {
      primary: '#213233',
      secondary: '#496163'
    },
    success: {
      main: '#2e8a4d'
    },
    warning: {
      main: '#d48724'
    },
    error: {
      main: '#c03d30'
    }
  },
  shape: {
    borderRadius: 16
  },
  typography: {
    fontFamily: [
      '"Space Grotesk"',
      '"Noto Sans SC"',
      '"PingFang SC"',
      '"Hiragino Sans GB"',
      'sans-serif'
    ].join(','),
    h3: {
      fontWeight: 700,
      letterSpacing: '-0.03em'
    },
    h4: {
      fontWeight: 700,
      letterSpacing: '-0.02em'
    },
    h5: {
      fontWeight: 650,
      letterSpacing: '-0.01em'
    },
    button: {
      fontWeight: 600,
      letterSpacing: '0.01em',
      textTransform: 'none'
    }
  },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: {
          margin: 0,
          minHeight: '100vh',
          backgroundImage: [
            'radial-gradient(circle at 10% 10%, rgba(191, 223, 204, 0.7), transparent 45%)',
            'radial-gradient(circle at 90% 20%, rgba(238, 211, 188, 0.7), transparent 42%)',
            'linear-gradient(145deg, #f7f2ea 0%, #efe6da 100%)'
          ].join(','),
          backgroundAttachment: 'fixed'
        },
        '#root': {
          minHeight: '100vh'
        }
      }
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          backdropFilter: 'blur(8px)'
        }
      }
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          backdropFilter: 'blur(8px)'
        }
      }
    },
    MuiButton: {
      styleOverrides: {
        root: {
          borderRadius: 12
        }
      }
    }
  }
});

export default theme;
