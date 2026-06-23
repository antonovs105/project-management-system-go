import type { ID, ProjectMember } from "../../types";

export function memberLabel(member: ProjectMember): string {
  const primary = member.name || member.username || member.email || member.handle || member.user_id;
  const secondary = member.email && member.email !== primary ? member.email : member.handle;
  return secondary ? `${primary} / ${secondary}` : primary;
}

export function projectMemberLabel(members: ProjectMember[], userID: ID | null | undefined): string {
  if (!userID) {
    return "Unassigned";
  }
  const member = members.find((item) => item.user_id === userID);
  return member ? memberLabel(member) : userID;
}
