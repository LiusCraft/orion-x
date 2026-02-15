import FeaturePlaceholder from '../components/FeaturePlaceholder.jsx';

export default function VoicebotDevicePage() {
  return (
    <FeaturePlaceholder
      title="Voicebots & Devices"
      description="Workspace for voicebot ownership, device binding, and entitlement attachment flow."
      scope={[
        'Visible to both admin and normal_user',
        'Built on auth-protected shell with unified error prompts',
        'Prepared for /api/v1/voicebots and /api/v1/devices endpoints'
      ]}
      nextSteps={['voicebot list', 'device binding editor', 'entitlement selector']}
    />
  );
}
