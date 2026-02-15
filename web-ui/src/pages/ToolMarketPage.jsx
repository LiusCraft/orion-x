import FeaturePlaceholder from '../components/FeaturePlaceholder.jsx';

export default function ToolMarketPage() {
  return (
    <FeaturePlaceholder
      title="Tool Market"
      description="Shared page for admin and normal_user to browse offers and manage entitlements."
      scope={[
        'Role-aware menu item visible to admin and normal_user',
        'Protected by auth gate and auto refresh token policy',
        'Prepared for tool-market and tool-repo API integration'
      ]}
      nextSteps={['list offers', 'activate flow', 'my entitlements panel']}
    />
  );
}
