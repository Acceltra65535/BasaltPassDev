import React, { useState, useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import Layout from '@features/user/components/Layout';
import { PButton, PSkeleton, PAlert, PBadge, PPageHeader, PCard } from '@ui';
import PTable, { PTableColumn, PTableAction } from '@ui/PTable';
import { teamApi, TeamMemberResponse } from '@api/user/team';
import { useI18n } from '@shared/i18n';

type TeamMember = TeamMemberResponse;

const TeamMembers: React.FC = () => {
  const { t, locale } = useI18n();
  const { id } = useParams<{ id: string }>();
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [currentUserRole, setCurrentUserRole] = useState<string>('');
  const [savingMemberId, setSavingMemberId] = useState<number | null>(null);

  useEffect(() => {
    if (id) {
      loadMembers();
    }
  }, [id]);

  const loadMembers = async () => {
    try {
      setLoading(true);
      setError(null);
      const teamId = parseInt(id!, 10);
      const [teamResponse, membersResponse] = await Promise.all([
        teamApi.getTeam(teamId),
        teamApi.getTeamMembers(teamId),
      ]);
      setMembers(membersResponse.data.data || []);
      setCurrentUserRole(teamResponse.data.data?.user_role || '');
    } catch (err: any) {
      setError(err.response?.data?.error || err.response?.data?.message || t('pages.teamMembers.errors.loadFailed'));
    } finally {
      setLoading(false);
    }
  };

  const getRoleBadge = (role: string) => {
    const roleVariants = {
      owner: 'error' as const,
      admin: 'info' as const,
      member: 'default' as const,
    };
    const roleNames = {
      owner: t('pages.teamDetail.roles.owner'),
      admin: t('pages.teamDetail.roles.admin'),
      member: t('pages.teamDetail.roles.member'),
    } as const;
    return (
      <PBadge variant={roleVariants[role as keyof typeof roleVariants] || 'default'}>
        {roleNames[role as keyof typeof roleNames] || role}
      </PBadge>
    );
  };

  const canManageMembers = currentUserRole === 'owner' || currentUserRole === 'admin';

  const handleRoleChange = async (member: TeamMember, role: 'owner' | 'admin' | 'member') => {
    if (!id || member.role === role) return;
    try {
      setSavingMemberId(member.id);
      await teamApi.updateMemberRole(parseInt(id, 10), member.id, { role });
      setMembers((prev) => prev.map((item) => (item.id === member.id ? { ...item, role } : item)));
    } catch (err: any) {
      window.alert(err.response?.data?.error || err.response?.data?.message || t('pages.teamMembers.errors.updateFailed'));
    } finally {
      setSavingMemberId(null);
    }
  };

  const handleRemoveMember = async (member: TeamMember) => {
    if (!id) return;
    if (!window.confirm(t('pages.teamMembers.confirmRemove'))) return;
    try {
      setSavingMemberId(member.id);
      await teamApi.removeMember(parseInt(id, 10), member.id);
      setMembers((prev) => prev.filter((item) => item.id !== member.id));
    } catch (err: any) {
      window.alert(err.response?.data?.error || err.response?.data?.message || t('pages.teamMembers.errors.removeFailed'));
    } finally {
      setSavingMemberId(null);
    }
  };

  if (loading) {
    return (
      <Layout>
        <div className="py-4">
          <PSkeleton.List items={4} />
        </div>
      </Layout>
    );
  }

  if (error) {
    return (
      <Layout>
        <PAlert variant="error" title={t('pages.teamMembers.errors.title')} message={error} />
      </Layout>
    );
  }

  return (
    <Layout>
      <div className="space-y-6">
        {/*  */}
        <PPageHeader
          title={t('pages.teamMembers.title')}
          description={t('pages.teamMembers.description')}
          backTo={`/teams/${id}`}
          backLabel={t('pages.teamMembers.actions.backToTeam')}
        />

        {/*  */}
        <PCard padding="none">
          <div className="px-6 py-4 border-b border-gray-200">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-medium text-gray-900">{t('pages.teamMembers.memberListTitle')}</h3>
              {canManageMembers && (
                <Link to={`/teams/invite/${id}`}>
                  <PButton variant="primary">{t('pages.teamMembers.actions.inviteMembers')}</PButton>
                </Link>
              )}
            </div>
          </div>
          <div className="overflow-x-auto">
            {(() => {
              const columns: PTableColumn<TeamMember>[] = [
                {
                  key: 'user',
                  title: t('pages.teamMembers.columns.user'),
                  render: (member) => (
                    <div className="flex items-center">
                      <div className="flex-shrink-0 h-10 w-10">
                        <div className="h-10 w-10 rounded-full bg-gray-300 flex items-center justify-center">
                          <span className="text-sm font-medium text-gray-700">
                            {member.user.nickname?.charAt(0) || member.user.email.charAt(0).toUpperCase()}
                          </span>
                        </div>
                      </div>
                      <div className="ml-4">
                        <div className="text-sm font-medium text-gray-900">
                          {member.user.nickname || t('pages.teamMembers.noNickname')}
                        </div>
                        <div className="text-sm text-gray-500">
                          {member.user.email}
                        </div>
                      </div>
                    </div>
                  )
                },
                {
                  key: 'role',
                  title: t('pages.teamMembers.columns.role'),
                  render: (member) => {
                    if (!canManageMembers || member.role === 'owner') {
                      return getRoleBadge(member.role);
                    }
                    return (
                      <select
                        value={member.role}
                        disabled={savingMemberId === member.id}
                        onChange={(event) => handleRoleChange(member, event.target.value as 'owner' | 'admin' | 'member')}
                        className="rounded-lg border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-700 shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500 disabled:cursor-not-allowed disabled:bg-gray-100 disabled:text-gray-500"
                      >
                        <option value="admin">{t('pages.teamDetail.roles.admin')}</option>
                        <option value="member">{t('pages.teamDetail.roles.member')}</option>
                      </select>
                    );
                  },
                },
                {
                  key: 'joined_at',
                  title: t('pages.teamMembers.columns.joinedAt'),
                  sortable: true,
                  sorter: (a, b) => new Date(a.joined_at).getTime() - new Date(b.joined_at).getTime(),
                  render: (member) => new Date(member.joined_at).toLocaleDateString(locale),
                },
              ];

              const actions: PTableAction<TeamMember>[] = canManageMembers
                ? [
                    {
                      key: 'remove',
                      label: t('pages.teamMembers.actions.remove'),
                      variant: 'danger',
                      size: 'sm',
                      onClick: handleRemoveMember,
                    },
                  ]
                : [];

              return (
                <PTable
                  columns={columns}
                  data={members}
                  rowKey={(row) => row.id}
                  actions={actions}
                  emptyText={t('pages.teamMembers.empty')}
                  striped
                />
              );
            })()}
          </div>
        </PCard>
      </div>
    </Layout>
  );
};

export default TeamMembers;
