import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import Layout from '@features/user/components/Layout';
import { PCard, PButton, PInput, PTextarea, PPageHeader, PAlert } from '@ui';
import { teamApi, CreateTeamRequest } from '@api/user/team';
import { UserGroupIcon, DocumentTextIcon, PhotoIcon } from '@heroicons/react/24/outline';
import { ROUTES } from '@constants';
import { useI18n } from '@shared/i18n';

const CreateTeam: React.FC = () => {
  const { t } = useI18n();
  const navigate = useNavigate();
  const [formData, setFormData] = useState<CreateTeamRequest>({
    name: '',
    description: '',
    avatar_url: '',
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value,
    }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!formData.name.trim()) {
      setError(t('pages.teamCreate.errors.nameRequired'));
      return;
    }

    try {
      setLoading(true);
      setError(null);
      
      await teamApi.createTeam(formData);
      
      // 
      navigate(ROUTES.user.teams, { 
        state: { message: t('pages.teamCreate.messages.createSuccess') }
      });
    } catch (err: any) {
      setError(err.response?.data?.message || t('pages.teamCreate.errors.createFailed'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Layout>
      <div className="space-y-6">
        <PPageHeader
          title={t('pages.teamCreate.title')}
          description={t('pages.teamCreate.description')}
          backTo={ROUTES.user.teams}
        />

        <PCard variant="bordered" size="lg">
          <form onSubmit={handleSubmit} className="space-y-8">
            {error && (
              <PAlert variant="error" title={t('pages.teamCreate.errors.title')} message={error} />
            )}

            <div className="space-y-2">
              <label htmlFor="name" className="flex items-center text-sm font-semibold text-gray-700">
                <UserGroupIcon className="h-5 w-5 mr-2 text-indigo-500" />
                {t('pages.teamCreate.fields.name')} <span className="text-red-500 ml-1">*</span>
              </label>
              <PInput
                type="text"
                id="name"
                name="name"
                value={formData.name}
                onChange={handleInputChange}
                placeholder={t('pages.teamCreate.placeholders.name')}
                size="lg"
                required
              />
            </div>

            <div className="space-y-2">
              <label htmlFor="description" className="flex items-center text-sm font-semibold text-gray-700">
                <DocumentTextIcon className="h-5 w-5 mr-2 text-indigo-500" />
                {t('pages.teamCreate.fields.description')}
              </label>
              <PTextarea
                id="description"
                name="description"
                value={formData.description}
                onChange={handleInputChange}
                rows={4}
                size="lg"
                className="resize-none"
                placeholder={t('pages.teamCreate.placeholders.description')}
              />
            </div>

            <div className="space-y-2">
              <label htmlFor="avatar_url" className="flex items-center text-sm font-semibold text-gray-700">
                <PhotoIcon className="h-5 w-5 mr-2 text-indigo-500" />
                {t('pages.teamCreate.fields.avatarUrl')}
              </label>
              <PInput
                type="url"
                id="avatar_url"
                name="avatar_url"
                value={formData.avatar_url}
                onChange={handleInputChange}
                placeholder="https://example.com/avatar.png"
                size="lg"
              />
              {formData.avatar_url && (
                <div className="mt-2 flex items-center space-x-2">
                  <div className="w-8 h-8 rounded-full bg-gray-100 flex items-center justify-center">
                    <PhotoIcon className="h-4 w-4 text-gray-400" />
                  </div>
                  <span className="text-xs text-gray-500">{t('pages.teamCreate.avatarPreviewEnabled')}</span>
                </div>
              )}
            </div>

            <div className="flex justify-end space-x-4 pt-8 border-t border-gray-100">
              <PButton
                type="button"
                variant="secondary"
                onClick={() => navigate(ROUTES.user.teams)}
                size="lg"
              >
                {t('pages.teamCreate.actions.cancel')}
              </PButton>
              <PButton
                type="submit"
                variant="primary"
                disabled={loading}
                loading={loading}
                size="lg"
              >
                {t('pages.teamCreate.actions.create')}
              </PButton>
            </div>
          </form>
        </PCard>
      </div>
    </Layout>
  );
};

export default CreateTeam;
