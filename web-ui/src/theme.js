import { createTheme } from '@mui/material/styles';

const theme = createTheme({
  palette: {
    mode: 'dark',
    background: {
      default: '#0E1116',
      paper: '#141A22'
    },
    primary: {
      main: '#4DA3FF'
    },
    secondary: {
      main: '#7B61FF'
    },
    success: {
      main: '#38D996'
    },
    warning: {
      main: '#FFB020'
    },
    error: {
      main: '#FF6B6B'
    }
  },
  shape: {
    borderRadius: 12
  },
  typography: {
    fontFamily: [
      'Inter',
      'PingFang SC',
      'Noto Sans SC',
      'Microsoft YaHei',
      'Helvetica Neue',
      'Arial',
      'sans-serif'
    ].join(',')
  },
  components: {
    MuiButton: {
      styleOverrides: {
        root: {
          textTransform: 'none'
        }
      }
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          backgroundImage: 'none'
        }
      }
    }
  }
});

export default theme;
