const configuredBaseUrl = import.meta.env.VITE_MANAGER_API_BASE_URL;

export const managerApiBaseUrl =
  configuredBaseUrl !== undefined
    ? configuredBaseUrl
    : import.meta.env.DEV
      ? ''
      : 'http://127.0.0.1:8081';
