package mysql

// Explicit business queries live here; services own transaction boundaries and
// state decisions. Keeping database/sql at that boundary avoids a wrapper for
// each statement while keeping SQL centralized and independently reviewable.
const (
	ActivitySelect = `SELECT features,expires_at FROM user_activity_profiles WHERE user_id=?`

	SuggestionSelect = `SELECT status,COALESCE(suggestions,JSON_ARRAY()) FROM experience_suggestion_jobs WHERE id=? AND user_id=?`

	CreateBottleInsert = `INSERT INTO bottles(id,owner_id,episode_raw,target_hint) VALUES (?,?,?,?)`

	EditBottleUpdate = `UPDATE bottles SET episode_raw=?,episode_title=?,episode_confirmed=?,target_hint=?,target_rules=?,content_version=? WHERE id=?`

	BottleActionUpdate = `UPDATE bottles SET status='paused' WHERE id=?`

	BottleActionDelete = `DELETE FROM active_search_slots WHERE bottle_id=?`

	BottleActionInsert = `INSERT INTO active_search_slots(user_id,bottle_id) VALUES (?,?)`

	BottleActionLockUser = `SELECT id FROM users WHERE id=? FOR UPDATE`

	BottleActionSelect = `SELECT bottle_id FROM active_search_slots WHERE user_id=? ORDER BY bottle_id`

	BottleActionCount = `SELECT COUNT(*) FROM active_search_slots WHERE user_id=?`

	BottleActionUpdate2 = `UPDATE bottles SET status='searching',failure_reason=NULL,search_round=?,content_version=?,target_rules=?,target_hint=?,launched_at=COALESCE(launched_at,UTC_TIMESTAMP(6)) WHERE id=?`

	BottleDetailSelect = `SELECT c.id,c.status,c.updated_at,EXISTS(SELECT 1 FROM notifications n WHERE n.user_id=? AND n.resource_id=c.id AND n.read_at IS NULL) FROM connections c WHERE c.bottle_id=? ORDER BY c.id`

	SearchStatusSelect = `SELECT COUNT(*) FROM match_invitations WHERE bottle_id=? AND search_round=?`

	AppealBottleUpdate = `UPDATE moderation_reviews SET status='pending',appeal_text=?,appealed_at=UTC_TIMESTAMP(6) WHERE resource_type='bottle' AND resource_id=? AND content_version=?`

	InviteChatSelect = `SELECT id,status FROM chat_invitations WHERE connection_id=?`

	InviteChatInsert = `INSERT INTO chat_invitations(id,connection_id) VALUES (?,?)`

	ChatStatusSelect = `SELECT i.id,i.status,s.id,s.status FROM chat_invitations i LEFT JOIN chat_sessions s ON s.invitation_id=i.id WHERE i.connection_id=?`

	DecideChatSelect = `SELECT connection_id FROM chat_invitations WHERE id=?`

	DecideChatSelect2 = `SELECT status FROM chat_invitations WHERE id=?`

	DecideChatUpdate = `UPDATE chat_invitations SET status=?,decided_at=UTC_TIMESTAMP(6) WHERE id=?`

	DecideChatInsert = `INSERT INTO chat_sessions(id,connection_id,invitation_id) VALUES (?,?,?)`

	ReportSelect = `SELECT EXISTS(SELECT 1 FROM messages WHERE id=? AND connection_id=? AND delivery_status='delivered')`

	ReportInsert = `INSERT INTO abuse_reports(id,reporter_id,connection_id,message_id,reason) VALUES (?,?,?,?,?)`

	BlockInsert = `INSERT INTO user_blocks(blocker_id,blocked_id) VALUES (?,?) ON DUPLICATE KEY UPDATE blocked_id=VALUES(blocked_id)`

	ConnectionDetailSelect = `SELECT experience_snapshot FROM match_invitations WHERE id=?`

	MessagesSelect = `SELECT id,body,kind,delivery_status,content_version,sender_id,created_at FROM messages WHERE connection_id=? AND (delivery_status='delivered' OR sender_id=?) AND (?='' OR id<?) ORDER BY id DESC LIMIT ?`

	SendMessageSelect = `SELECT id,body,kind,delivery_status FROM messages WHERE connection_id=? AND sender_id=? AND idempotency_key=?`

	SendMessageSelect2 = `SELECT EXISTS(SELECT 1 FROM messages WHERE connection_id=? AND kind='reply')`

	SendMessageSelect3 = `SELECT EXISTS(SELECT 1 FROM chat_sessions WHERE connection_id=? AND status='active')`

	SendMessageSelect4 = `SELECT COALESCE(MAX(sequence_no),0)+1 FROM messages WHERE connection_id=?`

	SendMessageInsert = `INSERT INTO messages(id,connection_id,sender_id,sequence_no,kind,body,idempotency_key) VALUES (?,?,?,?,?,?,?)`

	EditMessageSelect = `SELECT connection_id FROM messages WHERE id=? AND sender_id=?`

	EditMessageSelect2 = `SELECT delivery_status,content_version FROM messages WHERE id=? FOR UPDATE`

	EditMessageUpdate = `UPDATE messages SET body=?,content_version=?,delivery_status='pending_moderation' WHERE id=?`

	EditMessageUpdate2 = `UPDATE messages SET delivery_status='pending_moderation' WHERE id=?`

	EditMessageUpdate3 = `UPDATE moderation_reviews SET appeal_text=?,appealed_at=UTC_TIMESTAMP(6),status='pending' WHERE resource_type='message' AND resource_id=? AND content_version=?`

	CloseConnectionUpdate = `UPDATE connections SET status='closed',closed_at=UTC_TIMESTAMP(6) WHERE id=?`

	CloseConnectionUpdate2 = `UPDATE chat_sessions SET status='closed',closed_at=UTC_TIMESTAMP(6) WHERE connection_id=? AND status='active'`

	CloseConnectionUpdate3 = `UPDATE chat_invitations SET status='declined',decided_at=UTC_TIMESTAMP(6) WHERE connection_id=? AND status='pending'`

	CloseConnectionSelect = `SELECT closed_at FROM connections WHERE id=?`

	FeedbackSelect = `SELECT EXISTS(SELECT 1 FROM messages WHERE connection_id=? AND kind='reply' AND delivery_status='delivered')`

	FeedbackInsert = `INSERT INTO connection_feedback(connection_id,seeker_id,result) VALUES (?,?,?)`

	FeedbackSelect2 = `SELECT result FROM connection_feedback WHERE connection_id=?`

	ExperiencesSelect = `SELECT id,title,body,confirmed_by_user,receive_open,disclosure,source,updated_at FROM experiences WHERE owner_id=? ORDER BY id DESC`

	SaveExperienceInsert = `INSERT INTO experiences(id,owner_id,title,body,confirmed_by_user,receive_open,disclosure) VALUES (?,?,?,?,1,?,?)`

	SaveExperienceSelect = `SELECT title,body,confirmed_by_user,receive_open,disclosure FROM experiences WHERE id=? AND owner_id=? FOR UPDATE`

	SaveExperienceUpdate = `UPDATE experiences SET title=?,body=?,confirmed_by_user=?,receive_open=?,disclosure=? WHERE id=?`

	SaveExperienceSelect2 = `SELECT id,title,body,confirmed_by_user,receive_open,disclosure,source,updated_at FROM experiences WHERE id=?`

	DeleteExperienceDelete = `DELETE FROM experiences WHERE id=? AND owner_id=?`

	BottleExperienceUpsert = `INSERT INTO experiences(id,owner_id,title,body,confirmed_by_user,receive_open,disclosure,source) VALUES (?,?,?,?,1,0,JSON_OBJECT('summary',true,'timeRange',false,'domain',false),'bottle') ON DUPLICATE KEY UPDATE title=IF(source='bottle',VALUES(title),title),body=IF(source='bottle',VALUES(body),body),confirmed_by_user=IF(source='bottle',1,confirmed_by_user),updated_at=IF(source='bottle',UTC_TIMESTAMP(6),updated_at)`

	NextInvitationSelect = `SELECT i.id,i.bottle_id,COALESCE(i.matched_experience_id,''),i.reason,i.expires_at,COALESCE(b.episode_title,''),b.episode_raw,COALESCE(b.target_hint,'') FROM match_invitations i JOIN bottles b ON b.id=i.bottle_id WHERE i.recipient_id=? AND i.status='pending' AND i.expires_at>UTC_TIMESTAMP(6) AND NOT EXISTS(SELECT 1 FROM user_blocks x WHERE (x.blocker_id=i.recipient_id AND x.blocked_id=b.owner_id) OR (x.blocker_id=b.owner_id AND x.blocked_id=i.recipient_id)) ORDER BY i.id LIMIT 1`

	DecideSelect = `SELECT bottle_id FROM match_invitations WHERE id=? AND recipient_id=?`

	DecideUpdate = `UPDATE match_invitations SET status=?,decided_at=UTC_TIMESTAMP(6) WHERE id=? AND recipient_id=? AND status='pending' AND expires_at>UTC_TIMESTAMP(6)`

	DecideSelect2 = `SELECT status,expires_at<=UTC_TIMESTAMP(6) FROM match_invitations WHERE id=?`

	DecideInsert = `INSERT INTO connections(id,bottle_id,invitation_id,seeker_id,responder_id,status) VALUES (?,?,?,?,?,'awaiting_first_reply')`

	DecideSelect3 = `SELECT id FROM connections WHERE invitation_id=?`

	HomeSelect = `SELECT (SELECT COUNT(*) FROM match_invitations i JOIN bottles b ON b.id=i.bottle_id WHERE recipient_id=? AND i.status='pending' AND expires_at>UTC_TIMESTAMP(6) AND NOT EXISTS(SELECT 1 FROM user_blocks x WHERE (x.blocker_id=i.recipient_id AND x.blocked_id=b.owner_id) OR (x.blocker_id=b.owner_id AND x.blocked_id=i.recipient_id))),(SELECT COUNT(*) FROM notifications WHERE user_id=? AND read_at IS NULL AND type IN ('first_reply_received','chat_message_received')),(SELECT COUNT(*) FROM bottles WHERE owner_id=? AND launched_at IS NOT NULL),(SELECT COUNT(*) FROM connections WHERE responder_id=?),(SELECT COUNT(*) FROM experiences WHERE owner_id=?)`

	HomeActiveBottles = `SELECT bottle_id FROM active_search_slots WHERE user_id=? ORDER BY bottle_id DESC`

	CabinetSelect = `SELECT b.id,b.id,'',COALESCE(b.episode_title,''),LEFT(b.episode_raw,120),b.status,'',b.updated_at,EXISTS(SELECT 1 FROM notifications n WHERE n.user_id=? AND n.read_at IS NULL AND (n.resource_id=b.id OR n.resource_id IN(SELECT c.id FROM connections c WHERE c.bottle_id=b.id))) FROM bottles b WHERE b.owner_id=? AND b.launched_at IS NOT NULL AND (?='' OR b.id<?) ORDER BY b.id DESC LIMIT ?`

	CabinetSelect2 = `SELECT c.id,b.id,c.id,COALESCE(b.episode_title,''),LEFT(b.episode_raw,120),b.status,c.status,c.updated_at,EXISTS(SELECT 1 FROM notifications n WHERE n.user_id=? AND n.read_at IS NULL AND n.resource_id=c.id) FROM connections c JOIN bottles b ON b.id=c.bottle_id WHERE c.responder_id=? AND (?='' OR c.id<?) ORDER BY c.id DESC LIMIT ?`

	NotificationsSelect = `SELECT id,type,resource_type,resource_id,title,read_at,created_at FROM notifications WHERE user_id=? AND (?=0 OR read_at IS NULL) AND (?='' OR id<?) ORDER BY id DESC LIMIT ?`

	ReadNotificationUpdate = `UPDATE notifications SET read_at=COALESCE(read_at,UTC_TIMESTAMP(6)) WHERE id=? AND user_id=?`

	ReadNotificationSelect = `SELECT id,type,resource_type,resource_id,title,read_at,created_at FROM notifications WHERE id=? AND user_id=?`

	CreateSliceSelect = `SELECT id,status FROM slice_drafts WHERE connection_id=?`

	CreateSliceUpdate = `UPDATE slice_drafts SET status='generating' WHERE id=?`

	CreateSliceInsert = `INSERT INTO slice_drafts(id,connection_id,author_id,consented_at) VALUES (?,?,?,UTC_TIMESTAMP(6))`

	SliceSelect = `SELECT connection_id,COALESCE(title,''),COALESCE(body,''),status FROM slice_drafts WHERE id=? AND author_id=?`

	EditSliceSelect = `SELECT status FROM slice_drafts WHERE id=? AND author_id=? FOR UPDATE`

	EditSliceUpdate = `UPDATE slice_drafts SET status='published',published_at=UTC_TIMESTAMP(6) WHERE id=?`

	EditSliceUpdate2 = `UPDATE slice_drafts SET title=COALESCE(?,title),body=COALESCE(?,body) WHERE id=?`

	ReviewSelect = `SELECT status,COALESCE(reason_code,''),appeal_text FROM moderation_reviews WHERE resource_type=? AND resource_id=? AND content_version=?`

	ReviewInsert = `INSERT INTO moderation_reviews(id,resource_type,resource_id,content_version,content_hash,status,reason_code,reviewed_at) VALUES (?,?,?,?,?,?,?,UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE status=VALUES(status),reason_code=VALUES(reason_code),reviewed_at=VALUES(reviewed_at)`

	MatchBottleUpdate = `UPDATE bottles SET status='draft',failure_reason='content_rejected' WHERE id=?`

	MatchBottleSelect = `SELECT e.id,e.owner_id,e.title,e.body,e.disclosure,COALESCE(p.features,JSON_OBJECT()) FROM experiences e LEFT JOIN user_activity_profiles p ON p.user_id=e.owner_id WHERE e.confirmed_by_user=1 AND e.receive_open=1 AND e.owner_id<>? AND e.id>? AND NOT EXISTS(SELECT 1 FROM match_invitations i WHERE i.bottle_id=? AND i.recipient_id=e.owner_id) AND NOT EXISTS(SELECT 1 FROM user_blocks x WHERE (x.blocker_id=? AND x.blocked_id=e.owner_id) OR (x.blocker_id=e.owner_id AND x.blocked_id=?)) ORDER BY e.id LIMIT 200`

	MatchBottleSelect2 = `SELECT title,body,disclosure FROM experiences WHERE id=? AND owner_id=? AND receive_open=1 AND confirmed_by_user=1 FOR SHARE`

	MatchBottleInsert = `INSERT INTO match_invitations(id,bottle_id,search_round,recipient_id,matched_experience_id,experience_snapshot,reason,expires_at) VALUES (?,?,?,?,?,?,?,UTC_TIMESTAMP(6)+INTERVAL 72 HOUR)`

	MatchBottleSelect3 = `SELECT COALESCE(SUM(status='pending'),0),COALESCE(SUM(status='accepted'),0) FROM match_invitations WHERE bottle_id=? AND search_round=?`

	MatchBottleUpdate2 = `UPDATE bottles SET status=?,failure_reason=NULLIF(?,'') WHERE id=?`

	ModerateMessageSelect = `SELECT connection_id,body,delivery_status,content_version FROM messages WHERE id=?`

	ModerateMessageSelect2 = `SELECT sender_id,kind,delivery_status,content_version FROM messages WHERE id=? FOR UPDATE`

	ModerateMessageUpdate = `UPDATE messages SET delivery_status=?,delivered_at=IF(?='delivered',UTC_TIMESTAMP(6),NULL) WHERE id=?`

	ModerateMessageUpdate2 = `UPDATE connections SET status='replied' WHERE id=?`

	ModerateMessageUpdate3 = `UPDATE connections SET updated_at=UTC_TIMESTAMP(6) WHERE id=?`

	GenerateSliceSelect = `SELECT connection_id,author_id,status FROM slice_drafts WHERE id=?`

	GenerateSliceSelect2 = `SELECT body FROM messages WHERE connection_id=? AND sender_id=? AND kind='reply' AND delivery_status='delivered' ORDER BY sequence_no`

	GenerateSliceUpdate = `UPDATE slice_drafts SET title=?,body=?,status='ready' WHERE id=? AND status='generating'`

	SearchSelect = `SELECT response FROM external_api_cache WHERE provider='zhihu' AND capability='zhihu_search' AND request_hash=? AND expires_at>UTC_TIMESTAMP(6)`

	SearchInsert = `INSERT INTO external_api_account_usage(account_key,provider,capability,usage_date) VALUES (?,'zhihu','zhihu_search',?) ON DUPLICATE KEY UPDATE account_key=VALUES(account_key)`

	SearchUpdate = `UPDATE external_api_account_usage SET request_count=request_count+1 WHERE account_key=? AND provider='zhihu' AND capability='zhihu_search' AND usage_date=? AND request_count<?`

	SearchInsert2 = `INSERT INTO external_api_usage(user_id,provider,capability,usage_date,request_count) VALUES (?,'zhihu','zhihu_search',?,1) ON DUPLICATE KEY UPDATE request_count=request_count+1`

	SearchInsert3 = `INSERT INTO external_api_cache(provider,capability,request_hash,response,expires_at) VALUES ('zhihu','zhihu_search',?,?,UTC_TIMESTAMP(6)+INTERVAL 24 HOUR) ON DUPLICATE KEY UPDATE response=VALUES(response),expires_at=VALUES(expires_at)`

	IntegrationStatusSelect = `SELECT COALESCE(SUM(request_count),0) FROM external_api_account_usage WHERE account_key=? AND provider='zhihu' AND capability='zhihu_search' AND usage_date=?`

	IntegrationStatusSelect2 = `SELECT status,last_synced_at,oauth_expires_at IS NULL OR oauth_expires_at<=UTC_TIMESTAMP(6) FROM zhihu_integrations WHERE user_id=?`

	ConnectZhihuInsert = `INSERT INTO zhihu_integrations(user_id,oauth_token_ciphertext,oauth_expires_at,status) VALUES (?,?,?,'connected') ON DUPLICATE KEY UPDATE oauth_token_ciphertext=VALUES(oauth_token_ciphertext),oauth_expires_at=VALUES(oauth_expires_at),status='connected'`

	DisconnectZhihuUpdate = `UPDATE zhihu_integrations SET oauth_token_ciphertext=NULL,oauth_expires_at=NULL,status='disconnected' WHERE user_id=?`

	DisconnectZhihuDelete = `DELETE FROM user_activity_profiles WHERE user_id=?`

	RefreshActivitySelect = `SELECT oauth_token_ciphertext,oauth_expires_at FROM zhihu_integrations WHERE user_id=? AND status='connected'`

	RefreshActivityUpdate = `UPDATE zhihu_integrations SET status='reauthorization_required',oauth_token_ciphertext=NULL WHERE user_id=?`

	RefreshActivitySelect2 = `SELECT oauth_token_ciphertext FROM zhihu_integrations WHERE user_id=? AND status='connected' FOR UPDATE`

	RefreshActivityInsert = `INSERT INTO user_activity_profiles(user_id,features,source_types,refreshed_at,expires_at) VALUES (?,?,JSON_ARRAY('public_content','followees'),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)+INTERVAL 6 HOUR) ON DUPLICATE KEY UPDATE features=VALUES(features),source_types=VALUES(source_types),refreshed_at=VALUES(refreshed_at),expires_at=VALUES(expires_at),profile_version=profile_version+1`

	RefreshActivityUpdate2 = `UPDATE zhihu_integrations SET last_synced_at=UTC_TIMESTAMP(6) WHERE user_id=?`

	ClaimSelect = `SELECT id,type,payload,attempts FROM outbox_jobs WHERE (status='pending' AND available_at<=UTC_TIMESTAMP(6)) OR (status='processing' AND locked_at<UTC_TIMESTAMP(6)-INTERVAL 5 MINUTE) ORDER BY available_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`

	ClaimUpdate = `UPDATE outbox_jobs SET status='processing',attempts=?,locked_at=UTC_TIMESTAMP(6),locked_by=? WHERE id=?`

	FinishSelect = `SELECT locked_by,status FROM outbox_jobs WHERE id=? FOR UPDATE`

	FinishUpdate = `UPDATE outbox_jobs SET status='succeeded',completed_at=UTC_TIMESTAMP(6),locked_by=NULL,locked_at=NULL,last_error=NULL WHERE id=?`

	FinishUpdate2 = `UPDATE outbox_jobs SET status='pending',available_at=UTC_TIMESTAMP(6)+INTERVAL ? SECOND,locked_by=NULL,locked_at=NULL,last_error=? WHERE id=?`

	FinishUpdate3 = `UPDATE outbox_jobs SET status='failed',completed_at=UTC_TIMESTAMP(6),locked_by=NULL,locked_at=NULL,last_error=? WHERE id=?`

	FinishUpdate4 = `UPDATE bottles SET status='search_error',failure_reason='matching_unavailable' WHERE id=?`

	FinishUpdate5 = `UPDATE messages SET delivery_status='moderation_failed' WHERE id=? AND content_version=? AND delivery_status='pending_moderation'`

	FinishUpdate6 = `UPDATE slice_drafts SET status='failed' WHERE id=? AND status='generating'`

	RecoverUpdate = `UPDATE match_invitations SET status='expired',decided_at=UTC_TIMESTAMP(6) WHERE status='pending' AND expires_at<=UTC_TIMESTAMP(6) ORDER BY expires_at LIMIT 100`

	RecoverSelect = `SELECT b.id,b.search_round FROM bottles b WHERE b.status='searching' AND NOT EXISTS(SELECT 1 FROM match_invitations i WHERE i.bottle_id=b.id AND i.search_round=b.search_round AND i.status='pending') AND NOT EXISTS(SELECT 1 FROM outbox_jobs j WHERE j.type='match_bottle' AND j.status IN ('pending','processing') AND JSON_UNQUOTE(JSON_EXTRACT(j.payload,'$.id'))=b.id AND JSON_EXTRACT(j.payload,'$.round')=b.search_round) ORDER BY b.id LIMIT 100`

	RecoverSelect2 = `SELECT z.user_id FROM zhihu_integrations z LEFT JOIN user_activity_profiles p ON p.user_id=z.user_id WHERE z.status='connected' AND z.oauth_expires_at>UTC_TIMESTAMP(6) AND (p.expires_at IS NULL OR p.expires_at<=UTC_TIMESTAMP(6)) AND NOT EXISTS(SELECT 1 FROM outbox_jobs j WHERE j.type='refresh_activity' AND j.status IN ('pending','processing') AND JSON_UNQUOTE(JSON_EXTRACT(j.payload,'$.user'))=z.user_id) ORDER BY z.user_id LIMIT 100`

	RecoverUpdate2 = `UPDATE zhihu_integrations SET status='reauthorization_required',oauth_token_ciphertext=NULL WHERE status='connected' AND oauth_expires_at<=UTC_TIMESTAMP(6)`

	RecoverDelete = `DELETE FROM external_api_cache WHERE expires_at<=UTC_TIMESTAMP(6) LIMIT 100`

	RecoverDelete2 = `DELETE FROM outbox_jobs WHERE status IN ('succeeded','failed') AND completed_at<UTC_TIMESTAMP(6)-INTERVAL 7 DAY LIMIT 100`
)
