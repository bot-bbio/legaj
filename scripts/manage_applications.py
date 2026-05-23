import sys
import os
import datetime
from openpyxl import load_workbook
from openpyxl.styles import Alignment, Border, Side

def add_application(file_path, company, role, location, link, status="Applied", resume="", cover_letter="", notes=""):
    if not os.path.exists(file_path):
        print(f"Error: Tracker spreadsheet does not exist at {file_path}. Run create_tracker.py first.")
        sys.exit(1)
        
    try:
        wb = load_workbook(file_path)
        ws = wb.active
        
        thin_border = Border(
            left=Side(style='thin', color='CCCCCC'),
            right=Side(style='thin', color='CCCCCC'),
            top=Side(style='thin', color='CCCCCC'),
            bottom=Side(style='thin', color='CCCCCC')
        )
        
        date_str = datetime.date.today().strftime("%Y-%m-%d")
        
        new_row = [
            company, role, location, date_str, link, status, resume, cover_letter, notes
        ]
        
        ws.append(new_row)
        
        # Style the new row
        row_idx = ws.max_row
        for col_idx in range(1, len(new_row) + 1):
            cell = ws.cell(row=row_idx, column=col_idx)
            cell.border = thin_border
            if col_idx in [4, 6]:  # Date, Status
                cell.alignment = Alignment(horizontal="center")
                
        wb.save(file_path)
        print(f"Successfully added application for {role} at {company} to tracker.")
    except Exception as e:
        print(f"Error writing to spreadsheet: {str(e)}")

def update_application_status(file_path, company, role, new_status, notes=None):
    if not os.path.exists(file_path):
        print(f"Error: Tracker spreadsheet does not exist at {file_path}.")
        sys.exit(1)
        
    try:
        wb = load_workbook(file_path)
        ws = wb.active
        
        found = False
        for row in range(2, ws.max_row + 1):
            row_company = ws.cell(row=row, column=1).value
            row_role = ws.cell(row=row, column=2).value
            
            # Match company and role (case-insensitive)
            if row_company and row_role and row_company.lower().strip() == company.lower().strip() and row_role.lower().strip() == role.lower().strip():
                ws.cell(row=row, column=6).value = new_status
                if notes:
                    current_notes = ws.cell(row=row, column=9).value or ""
                    ws.cell(row=row, column=9).value = f"{current_notes} | {notes}" if current_notes else notes
                found = True
                
        if found:
            wb.save(file_path)
            print(f"Updated status for {role} at {company} to '{new_status}'.")
        else:
            print(f"No application found matching {role} at {company}.")
    except Exception as e:
        print(f"Error updating spreadsheet: {str(e)}")

def scan_emails_and_sync(file_path, email_addr, password, imap_server):
    import imaplib
    import email
    from email.header import decode_header
    
    print(f"Connecting to IMAP server {imap_server}...")
    try:
        mail = imaplib.IMAP4_SSL(imap_server)
        mail.login(email_addr, password)
        mail.select("inbox")
        
        # Search for messages containing job keywords
        status, messages = mail.search(None, '(OR OR BODY "application" BODY "interview" SUBJECT "application" SUBJECT "interview")')
        
        if status != "OK":
            print("No matching emails found.")
            return
            
        wb = load_workbook(file_path)
        ws = wb.active
        
        email_ids = messages[0].split()
        print(f"Scanning {len(email_ids)} emails for application status updates...")
        
        # Scan last 20 emails to avoid latency/timeout issues
        updates_made = False
        for e_id in email_ids[-20:]:
            res, msg_data = mail.fetch(e_id, "(RFC822)")
            if res != "OK":
                continue
                
            for response_part in msg_data:
                if isinstance(response_part, tuple):
                    msg = email.message_from_bytes(response_part[1])
                    subject, encoding = decode_header(msg["Subject"])[0]
                    if isinstance(subject, bytes):
                        subject = subject.decode(encoding or "utf-8", errors="ignore")
                    
                    sender = msg.get("From", "")
                    body = ""
                    if msg.is_multipart():
                        for part in msg.walk():
                            if part.get_content_type() == "text/plain":
                                body = part.get_payload(decode=True).decode(errors="ignore")
                                break
                    else:
                        body = msg.get_payload(decode=True).decode(errors="ignore")
                        
                    subject_lower = subject.lower()
                    body_lower = body.lower()
                    
                    # Match sender or subject with entries in our sheet
                    for row in range(2, ws.max_row + 1):
                        comp = ws.cell(row=row, column=1).value
                        role = ws.cell(row=row, column=2).value
                        if not comp:
                            continue
                            
                        comp_lower = comp.lower().strip()
                        
                        # If email relates to this company
                        if comp_lower in subject_lower or comp_lower in sender.lower() or comp_lower in body_lower:
                            current_status = ws.cell(row=row, column=6).value
                            new_status = None
                            note_text = f"Email subject: {subject}"
                            
                            # Simple keyword classification
                            if "interview" in subject_lower or "schedule" in subject_lower or "chat with" in subject_lower:
                                new_status = "Interviewing"
                            elif "thank you for applying" in subject_lower or "received your application" in subject_lower or "application confirmation" in subject_lower:
                                if current_status == "Wishlist":
                                    new_status = "Applied"
                            elif "not moving forward" in body_lower or "unfortunately" in body_lower or "other candidates" in body_lower:
                                new_status = "Rejected"
                            elif "offer" in subject_lower or "congratulations" in subject_lower:
                                new_status = "Offer"
                                
                            if new_status and new_status != current_status:
                                ws.cell(row=row, column=6).value = new_status
                                curr_notes = ws.cell(row=row, column=9).value or ""
                                ws.cell(row=row, column=9).value = f"{curr_notes} | Auto-updated: {new_status} ({note_text})" if curr_notes else f"Auto-updated: {new_status} ({note_text})"
                                print(f"Auto-updated {role} at {comp} to '{new_status}' based on email: '{subject}'")
                                updates_made = True
                                
        if updates_made:
            wb.save(file_path)
            print("Spreadsheet updated successfully with email scanning matches.")
        else:
            print("No new status updates found from recent emails.")
            
        mail.close()
        mail.logout()
    except Exception as e:
        print(f"IMAP Email connection/sync failed: {str(e)}")

def list_applications_json(file_path):
    if not os.path.exists(file_path):
        print("[]")
        return
    try:
        wb = load_workbook(file_path)
        ws = wb.active
        apps = []
        for row in range(2, ws.max_row + 1):
            comp = ws.cell(row=row, column=1).value
            role = ws.cell(row=row, column=2).value
            loc = ws.cell(row=row, column=3).value
            date = ws.cell(row=row, column=4).value
            link = ws.cell(row=row, column=5).value
            status = ws.cell(row=row, column=6).value
            res = ws.cell(row=row, column=7).value
            cl = ws.cell(row=row, column=8).value
            notes = ws.cell(row=row, column=9).value
            if comp or role:
                apps.append({
                    "company": comp or "",
                    "role": role or "",
                    "location": loc or "",
                    "date": str(date) if date else "",
                    "link": link or "",
                    "status": status or "",
                    "resume": res or "",
                    "cover_letter": cl or "",
                    "notes": notes or ""
                })
        import json
        print(json.dumps(apps))
    except Exception as e:
        print("[]")

def main():
    if len(sys.argv) < 3:
        print("Usage:")
        print("  python manage_applications.py <tracker_path> add <company> <role> <location> <link> [resume] [cover_letter] [notes]")
        print("  python manage_applications.py <tracker_path> update <company> <role> <new_status> [notes]")
        print("  python manage_applications.py <tracker_path> sync <email> <password> <imap_server>")
        print("  python manage_applications.py <tracker_path> list")
        sys.exit(1)
        
    tracker_path = sys.argv[1]
    action = sys.argv[2].lower()
    
    if action == "list":
        list_applications_json(tracker_path)
        
    elif action == "add":
        if len(sys.argv) < 7:
            print("Missing arguments for 'add' command.")
            sys.exit(1)
        company = sys.argv[3]
        role = sys.argv[4]
        location = sys.argv[5]
        link = sys.argv[6]
        resume = sys.argv[7] if len(sys.argv) > 7 else ""
        cover_letter = sys.argv[8] if len(sys.argv) > 8 else ""
        notes = sys.argv[9] if len(sys.argv) > 9 else ""
        add_application(tracker_path, company, role, location, link, "Applied", resume, cover_letter, notes)
        
    elif action == "update":
        if len(sys.argv) < 6:
            print("Missing arguments for 'update' command.")
            sys.exit(1)
        company = sys.argv[3]
        role = sys.argv[4]
        new_status = sys.argv[5]
        notes = sys.argv[6] if len(sys.argv) > 6 else None
        update_application_status(tracker_path, company, role, new_status, notes)
        
    elif action == "sync":
        if len(sys.argv) < 6:
            print("Missing arguments for 'sync' command.")
            sys.exit(1)
        email_addr = sys.argv[3]
        password = sys.argv[4]
        imap_server = sys.argv[5]
        scan_emails_and_sync(tracker_path, email_addr, password, imap_server)
        
    else:
        print(f"Unknown action '{action}'")
        sys.exit(1)

if __name__ == "__main__":
    main()
