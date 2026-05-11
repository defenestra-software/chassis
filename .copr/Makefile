.PHONY: srpm

srpm:
	dnf -y install golang git tar make
	cd $(outdir) && bash $(CURDIR)/deploy/rpm/make_srpm.sh
