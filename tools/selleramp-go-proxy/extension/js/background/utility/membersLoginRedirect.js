import{declarativeNetRequest,ruleActionType}from"../constants/declarativeNetRequestConstants.js";import{ruleNameIDs}from"../constants/ruleNameIDs.js";
// Only intercept third-party member login hosts — never the local/proxy SAS panel itself.
const addMembersLoginRedirect=async()=>declarativeNetRequest.updateDynamicRules({
  removeRuleIds:[ruleNameIDs.showAMZToolsConsultantLoginPage],
  addRules:[{
    id:ruleNameIDs.showAMZToolsConsultantLoginPage,
    priority:1,
    action:{type:ruleActionType.REDIRECT,redirect:{extensionPath:"/auth-required.html"}},
    condition:{
      urlFilter:"||members.amztoolsconsultant.com/",
      resourceTypes:[declarativeNetRequest.ResourceType.SUB_FRAME]
    }
  }]
});
export{addMembersLoginRedirect};
